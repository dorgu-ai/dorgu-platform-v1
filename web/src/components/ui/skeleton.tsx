import { cn } from '@/lib/utils'

/**
 * Skeletons rather than spinners, per the plan. A spinner says "something is
 * happening"; a skeleton says "this is the shape of what is coming", which on a
 * table means the layout does not jump when the data lands.
 */
export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-line', className)} />
}

/** A table-shaped placeholder for the first paint of a view. */
export function SkeletonRows({ rows = 6, columns = 5 }: { rows?: number; columns?: number }) {
  return (
    <div className="divide-y divide-line" aria-hidden="true">
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div key={rowIndex} className="flex items-center gap-6 px-4 py-3">
          {Array.from({ length: columns }).map((_, columnIndex) => (
            <Skeleton
              key={columnIndex}
              className={cn('h-3', columnIndex === 0 ? 'w-40' : 'w-20')}
            />
          ))}
        </div>
      ))}
    </div>
  )
}
