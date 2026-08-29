import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table'
import { useVirtualizer } from '@tanstack/react-virtual'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { useRef, useState } from 'react'

import { cn } from '@/lib/utils'

// React Compiler is not enabled in this build, so its warning about
// useReactTable returning unmemoizable functions describes a hazard that cannot
// occur here. If the compiler is ever turned on, delete this and read the
// warning: it is a real constraint of TanStack Table, not a false positive.
/* eslint-disable react-hooks/incompatible-library */

/**
 * Extra column metadata this table understands.
 *
 * `width` is a CSS grid track. The table is a grid rather than a `<table>`
 * because virtualised rows have to be absolutely positioned, and a positioned
 * `<tr>` loses the column widths a table element was giving it.
 */
export interface ColumnMeta {
  width: string
  /** Right-align numeric columns so counts line up down the page. */
  numeric?: boolean
}

/** Rows must carry a stable id so the virtualiser does not re-key on a push. */
export interface Identifiable {
  id: string
}

/**
 * DataTable virtualises its rows.
 *
 * This is the answer to "smooth with a lot of data" from the plan: only the rows
 * in view are in the DOM, so an incident feed at the 500-row cap costs the same
 * to render as one at ten. It matters most on exactly the cluster where the
 * dashboard is being read for real, which is the one having a bad hour.
 */
export function DataTable<TData extends Identifiable>({
  columns,
  data,
  initialSorting = [],
  estimatedRowHeight = 44,
  emptyState,
  flashedIds,
  rowDetail,
  ariaLabel,
}: {
  /**
   * The column value type is left open.
   *
   * `createColumnHelper` returns a differently parameterised ColumnDef per
   * column, and a heterogeneous list of those is not assignable to
   * `ColumnDef<TData, unknown>[]`. This is the signature TanStack's own docs
   * use, and narrowing it would only move the cast to every call site.
   */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  columns: ColumnDef<TData, any>[]
  data: TData[]
  initialSorting?: SortingState
  estimatedRowHeight?: number
  emptyState?: React.ReactNode
  /** Row ids that just changed, flashed once so a live update is noticeable. */
  flashedIds?: ReadonlySet<string>
  /**
   * Extra content for an expanded row, rendered below its cells at the full
   * width of the table.
   *
   * It is a slot on the table rather than something a cell renders, because a
   * tall cell drags every other cell in its row to the vertical centre: put a
   * remediation plan inside the first column and the phase badge floats halfway
   * down the page, level with nothing. Rendering it under the cells keeps the
   * row aligned and gives the content the whole width instead of one column's.
   *
   * The virtualiser measures the result, so an expanded row can be any height
   * without the list mispositioning the rows after it.
   */
  rowDetail?: (row: TData) => React.ReactNode
  ariaLabel: string
}) {
  const [sorting, setSorting] = useState<SortingState>(initialSorting)
  const scrollRef = useRef<HTMLDivElement>(null)

  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getRowId: (row) => row.id,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  const rows = table.getRowModel().rows

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => estimatedRowHeight,
    // A few rows either side of the viewport, so a fast scroll does not show
    // blank space before the next batch renders.
    overscan: 12,
  })

  // Derived on every render rather than memoised.
  //
  // useReactTable returns the same object for the life of the component: it
  // mutates a ref rather than replacing it. So a memo keyed on `table` would
  // compute once at mount and never again, and a later change that made a column
  // conditional would silently keep the old track list. This is a map and a join
  // over a handful of columns, which is not worth a stale-value trap.
  const visibleColumns = table.getVisibleLeafColumns()
  const gridTemplate = visibleColumns
    .map((column) => (column.columnDef.meta as ColumnMeta | undefined)?.width ?? '1fr')
    .join(' ')

  const virtualRows = virtualizer.getVirtualItems()

  return (
    <div className="flex min-h-0 flex-1 flex-col" role="table" aria-label={ariaLabel}>
      <div
        className="grid shrink-0 items-center gap-4 border-b border-line bg-surface-sunken px-4 py-2"
        style={{ gridTemplateColumns: gridTemplate }}
        role="row"
      >
        {table.getHeaderGroups()[0]?.headers.map((header) => {
          const meta = header.column.columnDef.meta as ColumnMeta | undefined
          const sortDirection = header.column.getIsSorted()
          const sortable = header.column.getCanSort()

          return (
            <div
              key={header.id}
              role="columnheader"
              aria-sort={
                sortDirection === 'asc'
                  ? 'ascending'
                  : sortDirection === 'desc'
                    ? 'descending'
                    : undefined
              }
              className={cn(
                'text-[11px] font-semibold tracking-wide text-ink-faint uppercase',
                meta?.numeric && 'text-right',
              )}
            >
              {sortable ? (
                <button
                  type="button"
                  onClick={header.column.getToggleSortingHandler()}
                  className={cn(
                    'inline-flex items-center gap-1 hover:text-ink',
                    meta?.numeric && 'flex-row-reverse',
                  )}
                >
                  {flexRender(header.column.columnDef.header, header.getContext())}
                  {sortDirection === 'asc' && <ChevronUp className="size-3" />}
                  {sortDirection === 'desc' && <ChevronDown className="size-3" />}
                </button>
              ) : (
                flexRender(header.column.columnDef.header, header.getContext())
              )}
            </div>
          )
        })}
      </div>

      {rows.length === 0 ? (
        <div className="min-h-0 flex-1 overflow-auto">{emptyState}</div>
      ) : (
        <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto">
          <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
            {virtualRows.map((virtualRow) => {
              const row = rows[virtualRow.index]
              if (!row) return null

              const detail = rowDetail?.(row.original)

              return (
                <div
                  key={row.id}
                  // A rowgroup, so the cells and any detail below them are two
                  // rows rather than one row with a stray child inside it.
                  role="rowgroup"
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  className={cn(
                    'absolute top-0 left-0 w-full border-b border-line',
                    flashedIds?.has(row.id) && 'flash-on-change',
                  )}
                  style={{ transform: `translateY(${virtualRow.start}px)` }}
                >
                  <div
                    role="row"
                    className="grid w-full items-center gap-4 px-4 py-2.5 hover:bg-surface-raised"
                    style={{ gridTemplateColumns: gridTemplate }}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const meta = cell.column.columnDef.meta as ColumnMeta | undefined
                      return (
                        <div
                          key={cell.id}
                          role="cell"
                          className={cn('min-w-0 text-xs', meta?.numeric && 'text-right')}
                        >
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </div>
                      )
                    })}
                  </div>

                  {detail && (
                    <div role="row">
                      {/*
                        One cell spanning every column. Without aria-colspan a
                        screen reader in table mode sees a row with one cell
                        against a header that promises several, and reports the
                        table as ragged.
                      */}
                      <div role="cell" aria-colspan={visibleColumns.length} className="px-4 pb-3">
                        {detail}
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
