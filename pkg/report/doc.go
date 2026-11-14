// Package report computes Dorgu's KPI summary from the operator's own records.
//
// It is the one implementation behind `dorgu report` and the dashboard's
// Overview page, so the two cannot disagree about what a number means. It takes
// the operator's api/v1 types (RemediationAction, IncidentMemory), core/v1
// Events, a clock and a window, and returns a Doc. It reads no cluster, holds no
// client and does no I/O: the same inputs and the same clock give the same Doc.
//
// # What it refuses to do
//
// It never reports a zero as if it were a measurement. A ratio or percentile with
// no sample is nil with NoData true, a heal count is nil when the incidents it
// depends on could not be read, and KPIs a cluster cannot report at all are
// listed under NotMeasurable. A caller that could not read an input says so in
// Inputs.Errors (or leaves EventsRead false), and the sections that depend on it
// become no data with a reason.
package report
