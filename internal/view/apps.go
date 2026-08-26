/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package view

import (
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/workload"
)

// AppsInput is everything the Apps view is built from.
type AppsInput struct {
	Personas    []*dorguv1.ApplicationPersona
	Deployments []*appsv1.Deployment
	Pods        []*corev1.Pod
	Incidents   []*dorguv1.IncidentMemory

	// Namespace scopes the view. Empty means every namespace.
	Namespace string
	// ExcludedNamespaces are skipped when scanning for unmonitored
	// Deployments, so forty control-plane Deployments do not bury three real
	// apps. It is ignored when Namespace is set: an explicit namespace is an
	// explicit request.
	ExcludedNamespaces map[string]bool

	Readiness Readiness
}

// BuildApps projects personas and Deployments into the Apps view.
//
// Two kinds of row come out, and the difference is the point of the view:
//
//   - a persona row, which Dorgu watches and can act on
//   - an unmonitored Deployment, which Dorgu is blind to
//
// The second kind exists because presenting only what the tool can see is how
// `dorgu health` came to report "everything fine" on a cluster with three broken
// apps it had never been told about. A blind spot has to be named on the same
// screen as the things that are working, with the command that fixes it.
func BuildApps(in AppsInput) AppsPayload {
	deployments := filterNamespace(in.Deployments, in.Namespace)
	byNamespace := deploymentsByNamespace(deployments)
	incidentIndex := indexIncidents(in.Incidents)
	podIndex := podsByNamespace(filterPodNamespace(in.Pods, in.Namespace))

	apps := make([]App, 0, len(in.Personas)+len(deployments))
	matched := map[string]bool{}

	for _, persona := range in.Personas {
		if in.Namespace != "" && persona.Namespace != in.Namespace {
			continue
		}
		app, claimed := personaApp(persona, byNamespace[persona.Namespace], podIndex[persona.Namespace], incidentIndex)
		for _, name := range claimed {
			matched[persona.Namespace+"/"+name] = true
		}
		apps = append(apps, app)
	}

	for _, deploy := range deployments {
		if matched[deploy.Namespace+"/"+deploy.Name] {
			continue
		}
		if in.Namespace == "" && in.ExcludedNamespaces[deploy.Namespace] {
			continue
		}
		apps = append(apps, unmonitoredApp(deploy, podIndex[deploy.Namespace]))
	}

	sortApps(apps)
	return AppsPayload{
		Apps:      apps,
		Summary:   summarise(apps),
		Readiness: in.Readiness,
	}
}

// personaApp builds a persona row, and reports which Deployment names in the
// namespace the persona claims. The claim list is what keeps a matched
// Deployment from also appearing as unmonitored, and it deliberately includes
// every candidate of an ambiguous match: an ambiguous persona covers all of
// them badly, but it does cover them, so listing them as unwatched would be a
// second wrong answer on top of the first.
func personaApp(
	persona *dorguv1.ApplicationPersona,
	deployments []*appsv1.Deployment,
	pods []*corev1.Pod,
	incidents incidentIndexByPersona,
) (App, []string) {
	appName := persona.Spec.Name
	if appName == "" {
		appName = persona.Name
	}

	app := App{
		ID:        SourcePersona + "/" + persona.Namespace + "/" + persona.Name,
		Source:    SourcePersona,
		Monitored: true,
		Namespace: persona.Namespace,
		Name:      persona.Name,
		AppName:   appName,
		Type:      persona.Spec.Type,
		Tier:      persona.Spec.Tier,
		Phase:     persona.Status.Phase,
		CreatedAt: persona.CreationTimestamp,
	}
	if persona.Status.LastUpdated != nil {
		app.LastUpdated = persona.Status.LastUpdated
	}
	if o := persona.Spec.Ownership; o != nil {
		app.Ownership = &AppOwnership{
			Team:       o.Team,
			Owner:      o.Owner,
			OnCall:     o.OnCall,
			Runbook:    o.Runbook,
			Repository: o.Repository,
		}
	}

	deploy, rung, err := workload.Resolve(deployments, appName)
	var claimed []string
	switch {
	case err != nil:
		var ambiguous *workload.AmbiguousError
		if asAmbiguous(err, &ambiguous) {
			app.Warnings = append(app.Warnings, ambiguous.Error())
			claimed = append(claimed, ambiguous.Candidates...)
		} else {
			app.Warnings = append(app.Warnings, err.Error())
		}
	case deploy == nil:
		app.Warnings = append(app.Warnings, fmt.Sprintf(
			"No Deployment in namespace %s matches persona name %q. Looked at: %s.",
			persona.Namespace, appName, workload.ChainDescription()))
	default:
		app.Workload = buildWorkload(deploy, appName, rung)
		claimed = append(claimed, deploy.Name)
	}

	app.Health = personaHealth(persona, deploy, pods, appName)
	app.Incidents = incidents.forPersona(persona)
	return app, claimed
}

// unmonitoredApp builds a row for a Deployment no persona covers.
func unmonitoredApp(deploy *appsv1.Deployment, pods []*corev1.Pod) App {
	issues := podIssuesFor(deploy, pods)
	return App{
		ID:        SourceDeployment + "/" + deploy.Namespace + "/" + deploy.Name,
		Source:    SourceDeployment,
		Monitored: false,
		Namespace: deploy.Namespace,
		Name:      deploy.Name,
		AppName:   deploy.Name,
		CreatedAt: deploy.CreationTimestamp,
		Workload:  buildWorkload(deploy, deploy.Name, ""),
		Health:    observedHealth(deploy, issues),
		ImportCommand: fmt.Sprintf("dorgu persona import -n %s --name %s",
			deploy.Namespace, deploy.Name),
		Warnings: []string{
			"No ApplicationPersona covers this Deployment, so Dorgu is not watching it: " +
				"no detection, no diagnosis, no remediation.",
		},
	}
}

// buildWorkload reads the facts a reader needs about a live Deployment: who
// owns it, what image is running, and which resource keys it actually sets.
func buildWorkload(deploy *appsv1.Deployment, appName, rung string) *Workload {
	ownership := workload.DetectOwner(deploy)
	w := &Workload{
		Kind:            "Deployment",
		Name:            deploy.Name,
		Namespace:       deploy.Namespace,
		MatchedBy:       rung,
		ManagedBy:       ownership.ManagedBy,
		ManagedByDetail: ownership.Detail,
		Replicas: WorkloadReplicas{
			Desired:   desiredReplicas(deploy),
			Ready:     deploy.Status.ReadyReplicas,
			Available: deploy.Status.AvailableReplicas,
			Updated:   deploy.Status.UpdatedReplicas,
		},
	}
	if c := workload.PickContainer(deploy, appName); c != nil {
		w.Container = c.Name
		w.Image = c.Image
		w.ObservedResources = observedResources(c)
	}
	return w
}

// personaHealth reconciles the operator's verdict with what the dashboard can
// see for itself, and never reports the more flattering of the two.
//
// Three cases, in this order:
//
//  1. The operator's verdict is worse than or equal to the live observation, so
//     the verdict stands. It is the better answer: the operator has history and
//     a diagnosis, and the dashboard has one moment.
//
//  2. The live observation is worse. The verdict is stale, and the observation
//     wins. This is the case that matters. A persona's status is written on a
//     reconcile interval, so an app that started crash-looping ten seconds ago
//     still carries a Healthy verdict, and rendering it green next to a
//     CrashLoopBackOff would be exactly the failure that made `dorgu health`
//     report "everything fine" on a broken cluster. The disagreement itself is
//     reported, because a stale verdict is a fact about the operator.
//
//  3. There is no verdict at all, which is normal with healthCheck disabled or
//     before the first reconcile. The dashboard says what it can see and labels
//     the reading as its own.
func personaHealth(
	persona *dorguv1.ApplicationPersona,
	deploy *appsv1.Deployment,
	pods []*corev1.Pod,
	appName string,
) AppHealth {
	issues := podIssuesFor(deploy, pods)

	if h := persona.Status.Health; h != nil && h.Status != "" {
		reported := AppHealth{
			Status:    h.Status,
			Source:    HealthSourcePersona,
			Message:   h.Message,
			LastCheck: h.LastCheck,
			PodIssues: mergePodIssues(issues, h.PodFailures),
		}
		if deploy == nil {
			return reported
		}

		observed := observedHealth(deploy, issues)
		if !overrides(observed.Status, reported.Status) {
			return reported
		}

		return AppHealth{
			Status:    observed.Status,
			Source:    HealthSourceObserved,
			Message:   observed.Message,
			LastCheck: h.LastCheck,
			PodIssues: reported.PodIssues,
			Disagreement: fmt.Sprintf(
				"The operator last reported %s (%q). The dashboard can see worse right now: %s",
				h.Status, h.Message, observed.Message),
		}
	}

	if deploy == nil {
		return AppHealth{
			Status: HealthUnknown,
			Source: HealthSourceNone,
			Message: fmt.Sprintf(
				"No Deployment resolved for persona name %q, and the persona reports no health.", appName),
		}
	}
	return observedHealth(deploy, issues)
}

// observedHealth derives a health reading from replica counts and container
// states, which is everything a reader can check for themselves with kubectl.
func observedHealth(deploy *appsv1.Deployment, issues []PodIssue) AppHealth {
	desired := desiredReplicas(deploy)
	ready := deploy.Status.ReadyReplicas

	health := AppHealth{Source: HealthSourceObserved, PodIssues: issues}
	switch {
	case len(issues) > 0:
		health.Status = HealthUnhealthy
		health.Message = fmt.Sprintf("%s (%d/%d replicas ready)", issues[0].Reason, ready, desired)
	case desired == 0:
		// Scaled to zero is a decision, not a fault. Calling it Degraded would
		// put a red dot on a deliberately idle app.
		health.Status = HealthUnknown
		health.Message = "Scaled to 0 replicas."
	case ready == desired:
		health.Status = HealthHealthy
		health.Message = fmt.Sprintf("%d/%d replicas ready.", ready, desired)
	case ready == 0:
		health.Status = HealthUnhealthy
		health.Message = fmt.Sprintf("0/%d replicas ready.", desired)
	default:
		health.Status = HealthDegraded
		health.Message = fmt.Sprintf("%d/%d replicas ready.", ready, desired)
	}
	return health
}

// mergePodIssues folds the persona's recorded failures into the live ones,
// keeping live first and dropping duplicates by pod and container. The operator
// writes PodFailures at reconcile time and they can outlive the pod, so live
// observation wins where the two describe the same container.
func mergePodIssues(live []PodIssue, reported []dorguv1.PodFailure) []PodIssue {
	seen := make(map[string]bool, len(live))
	for _, i := range live {
		seen[i.PodName+"/"+i.Container] = true
	}
	out := live
	for _, f := range reported {
		if seen[f.PodName+"/"+f.Container] {
			continue
		}
		out = append(out, PodIssue{
			PodName:   f.PodName,
			Container: f.Container,
			Reason:    f.Reason,
			Message:   f.Message,
		})
	}
	return out
}

// observedResources reads a container's resource block, preserving which keys
// are set. An absent key stays an empty string rather than becoming a zero
// quantity, because "the workload has no CPU limit" and "the workload has a CPU
// limit of 0" are different facts.
func observedResources(container *corev1.Container) *dorguv1.ObservedResources {
	limits := resourceValues(container.Resources.Limits)
	requests := resourceValues(container.Resources.Requests)
	if limits == nil && requests == nil {
		return nil
	}
	return &dorguv1.ObservedResources{Limits: limits, Requests: requests}
}

func resourceValues(list corev1.ResourceList) *dorguv1.ResourceValues {
	out := &dorguv1.ResourceValues{}
	if qty, ok := list[corev1.ResourceCPU]; ok {
		out.CPU = qty.String()
	}
	if qty, ok := list[corev1.ResourceMemory]; ok {
		out.Memory = qty.String()
	}
	if out.CPU == "" && out.Memory == "" {
		return nil
	}
	return out
}

// desiredReplicas reads spec.replicas, whose nil means one rather than zero.
func desiredReplicas(deploy *appsv1.Deployment) int32 {
	if deploy.Spec.Replicas == nil {
		return 1
	}
	return *deploy.Spec.Replicas
}

// podIssuesFor collects the containers in a bad state across a Deployment's
// pods, newest-looking problem first is not attempted: the order is by pod name
// so a row does not reshuffle on every SSE push.
func podIssuesFor(deploy *appsv1.Deployment, pods []*corev1.Pod) []PodIssue {
	if deploy == nil || len(pods) == 0 {
		return nil
	}
	selector, err := metav1.LabelSelectorAsSelector(deploy.Spec.Selector)
	if err != nil || selector.Empty() {
		// An empty selector matches everything, which would attribute the whole
		// namespace's pods to this Deployment. Report nothing rather than
		// something wrong.
		return nil
	}

	var issues []PodIssue
	for _, pod := range pods {
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		issues = append(issues, podIssues(pod)...)
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].PodName != issues[j].PodName {
			return issues[i].PodName < issues[j].PodName
		}
		return issues[i].Container < issues[j].Container
	})
	return issues
}

// startupReasons are the waiting reasons that mean "not yet", not "broken".
// Everything else a container waits on is a fault worth showing.
var startupReasons = map[string]bool{
	"ContainerCreating": true,
	"PodInitializing":   true,
}

// podIssues reads one pod's container states.
//
// The rule is inclusive by design: any waiting reason that is not a startup
// reason counts, so a reason nobody anticipated (a new CRI error string, an
// admission webhook rejection) shows up as itself instead of being filtered out
// by an allowlist that predates it.
func podIssues(pod *corev1.Pod) []PodIssue {
	var out []PodIssue
	for _, cs := range pod.Status.ContainerStatuses {
		switch {
		case cs.State.Waiting != nil && !startupReasons[cs.State.Waiting.Reason]:
			out = append(out, PodIssue{
				PodName:      pod.Name,
				Container:    cs.Name,
				Reason:       cs.State.Waiting.Reason,
				Message:      cs.State.Waiting.Message,
				Phase:        string(pod.Status.Phase),
				RestartCount: cs.RestartCount,
			})
		case cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0:
			out = append(out, PodIssue{
				PodName:      pod.Name,
				Container:    cs.Name,
				Reason:       terminatedReason(cs.State.Terminated),
				Message:      cs.State.Terminated.Message,
				Phase:        string(pod.Status.Phase),
				RestartCount: cs.RestartCount,
			})
		case cs.LastTerminationState.Terminated != nil && cs.RestartCount > 0 &&
			cs.LastTerminationState.Terminated.Reason == "OOMKilled":
			// A container that was OOMKilled and came back is running now and
			// would otherwise look fine. It is the single most important thing
			// Dorgu detects, so it is reported while the restart count still
			// carries the evidence.
			out = append(out, PodIssue{
				PodName:      pod.Name,
				Container:    cs.Name,
				Reason:       "OOMKilled",
				Message:      "Container was OOMKilled and restarted.",
				Phase:        string(pod.Status.Phase),
				RestartCount: cs.RestartCount,
			})
		}
	}
	return out
}

func terminatedReason(t *corev1.ContainerStateTerminated) string {
	if t.Reason != "" {
		return t.Reason
	}
	return fmt.Sprintf("Exited with code %d", t.ExitCode)
}

// summarise counts the rows, so the header and the table cannot disagree.
func summarise(apps []App) AppsSummary {
	summary := AppsSummary{Total: len(apps)}
	namespaces := map[string]bool{}
	for _, app := range apps {
		if app.Monitored {
			summary.Monitored++
		} else {
			summary.Unmonitored++
			namespaces[app.Namespace] = true
		}
		switch app.Health.Status {
		case HealthHealthy:
			summary.Healthy++
		case HealthDegraded:
			summary.Degraded++
		case HealthUnhealthy:
			summary.Unhealthy++
		}
	}
	summary.UnmonitoredNamespaces = sortedKeys(namespaces)
	return summary
}

// sortApps orders rows so the screen is stable and the worst news is at the
// top: unhealthy before degraded, monitored before unmonitored at equal
// severity, then namespace and name.
func sortApps(apps []App) {
	sort.SliceStable(apps, func(i, j int) bool {
		a, b := apps[i], apps[j]
		if ra, rb := healthRank(a.Health.Status), healthRank(b.Health.Status); ra != rb {
			return ra < rb
		}
		if a.Monitored != b.Monitored {
			return a.Monitored
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
}

// overrides reports whether a live observation should replace the operator's
// recorded verdict.
//
// Only positive evidence of trouble qualifies. Degraded and Unhealthy are
// statements that something is wrong right now, and they win over a stale
// verdict that says otherwise. Unknown is not: it is the absence of a reading,
// which a workload scaled deliberately to zero also produces, and letting it
// override would put a grey badge on a Healthy idle app and call that honesty.
func overrides(observed, reported string) bool {
	switch observed {
	case HealthUnhealthy, HealthDegraded:
		return healthRank(observed) < healthRank(reported)
	default:
		return false
	}
}

func healthRank(status string) int {
	switch status {
	case HealthUnhealthy:
		return 0
	case HealthDegraded:
		return 1
	case HealthUnknown:
		return 2
	case HealthHealthy:
		return 3
	default:
		return 4
	}
}

func filterNamespace(deployments []*appsv1.Deployment, namespace string) []*appsv1.Deployment {
	if namespace == "" {
		return deployments
	}
	out := make([]*appsv1.Deployment, 0, len(deployments))
	for _, d := range deployments {
		if d.Namespace == namespace {
			out = append(out, d)
		}
	}
	return out
}

func filterPodNamespace(pods []*corev1.Pod, namespace string) []*corev1.Pod {
	if namespace == "" {
		return pods
	}
	out := make([]*corev1.Pod, 0, len(pods))
	for _, p := range pods {
		if p.Namespace == namespace {
			out = append(out, p)
		}
	}
	return out
}

func deploymentsByNamespace(deployments []*appsv1.Deployment) map[string][]*appsv1.Deployment {
	out := map[string][]*appsv1.Deployment{}
	for _, d := range deployments {
		out[d.Namespace] = append(out[d.Namespace], d)
	}
	for ns := range out {
		sort.Slice(out[ns], func(i, j int) bool { return out[ns][i].Name < out[ns][j].Name })
	}
	return out
}

func podsByNamespace(pods []*corev1.Pod) map[string][]*corev1.Pod {
	out := map[string][]*corev1.Pod{}
	for _, p := range pods {
		out[p.Namespace] = append(out[p.Namespace], p)
	}
	return out
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// asAmbiguous is errors.As narrowed to the one type this file cares about,
// kept as a helper so the switch above reads as prose.
func asAmbiguous(err error, target **workload.AmbiguousError) bool {
	if a, ok := err.(*workload.AmbiguousError); ok {
		*target = a
		return true
	}
	return false
}
