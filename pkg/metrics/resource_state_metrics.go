/*
Copyright 2026 The Kubernetes Authors.

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

package metrics

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gobuffalo/flect"

	"sigs.k8s.io/controller-tools/pkg/metrics/internal/config"
)

func toResourceStateMetrics(resource config.Resource) config.ResourceStateMetricsStore {
	resourceLabels := labelsToCEL(resource.LabelsFromPath, nil, config.PathKindScalar)
	store := config.ResourceStateMetricsStore{
		Group: resource.GroupVersionKind.Group, Version: resource.GroupVersionKind.Version,
		Kind: resource.GroupVersionKind.Kind, Resource: resource.ResourcePlural, Resolver: "cel",
	}
	if store.Resource == "" {
		store.Resource = strings.ToLower(flect.Pluralize(resource.GroupVersionKind.Kind))
	}

	prefix := ""
	if resource.MetricNamePrefix != nil && *resource.MetricNamePrefix != "" {
		prefix = *resource.MetricNamePrefix + "_"
	}
	for _, generator := range resource.Metrics {
		family := config.ResourceStateMetricsFamily{Name: prefix + generator.Name, Help: generator.Help}
		switch generator.Each.Type {
		case config.MetricTypeGauge:
			family.Metrics = []config.ResourceStateMetricsMetric{gaugeToCEL(*generator.Each.Gauge)}
		case config.MetricTypeInfo:
			family.Metrics = []config.ResourceStateMetricsMetric{infoToCEL(*generator.Each.Info)}
		case config.MetricTypeStateSet:
			family.Metrics = stateSetToCEL(*generator.Each.StateSet)
		}
		// RSM v0.1.0 mutates family labels when inheriting store labels for
		// every processed object. Put resource labels directly on each metric
		// to avoid duplicate labels on the second and subsequent objects.
		for i := range family.Metrics {
			family.Metrics[i].Labels = append(family.Metrics[i].Labels, resourceLabels...)
		}
		store.Families = append(store.Families, family)
	}
	return store
}

func gaugeToCEL(g config.MetricGauge) config.ResourceStateMetricsMetric {
	labels := labelsToCEL(g.LabelsFromPath, g.Path, g.PathKind)
	if g.LabelFromKey != "" {
		labels = append(labels, config.ResourceStateMetricsLabel{Name: g.LabelFromKey, Value: mapKeysCEL(g.Path)})
	}
	valuePath := append(append([]string{}, g.Path...), g.ValueFrom...)
	value := numericPathCEL("o", valuePath, g.ValueKind == config.ValueKindTimestamp)
	switch g.PathKind {
	case config.PathKindArray:
		value = guardedMapPathCEL(g.Path, g.ValueFrom, true, g.ValueKind == config.ValueKindTimestamp)
	case config.PathKindScalar, config.PathKindObject:
		guard, _ := guardedPathCEL("o", valuePath)
		if g.NilIsZero {
			value = fmt.Sprintf("%s ? %s : 0", guard, value)
		} else if len(valuePath) > 0 {
			value = fmt.Sprintf("%s ? [%s] : []", guard, value)
		}
	}
	return config.ResourceStateMetricsMetric{Labels: labels, Value: value}
}

func infoToCEL(i config.MetricInfo) config.ResourceStateMetricsMetric {
	labels := labelsToCEL(i.LabelsFromPath, i.Path, i.PathKind)
	if i.LabelFromKey != "" {
		labels = append(labels, config.ResourceStateMetricsLabel{Name: i.LabelFromKey, Value: mapKeysCEL(i.Path)})
	}
	value := "1"
	if len(i.Path) > 0 {
		guard, path := guardedPathCEL("o", i.Path)
		if i.PathKind == config.PathKindArray {
			value = fmt.Sprintf("%s ? %s.map(v, 1) : []", guard, path)
		} else {
			value = fmt.Sprintf("%s ? [1] : []", guard)
		}
	}
	return config.ResourceStateMetricsMetric{Labels: labels, Value: value}
}

func stateSetToCEL(s config.MetricStateSet) []config.ResourceStateMetricsMetric {
	metrics := make([]config.ResourceStateMetricsMetric, 0, len(s.List))
	for _, state := range s.List {
		labels := labelsToCEL(s.LabelsFromPath, s.Path, s.PathKind)
		labels = append(labels, config.ResourceStateMetricsLabel{Name: s.LabelName, Value: strconv.Quote(state)})
		guard, base := guardedPathCEL("o", s.Path)
		var value string
		if s.PathKind == config.PathKindArray {
			actual := pathCEL("v", s.ValueFrom)
			value = fmt.Sprintf("%s ? %s.map(v, int(%s == %s ? 1 : 0)) : []", guard, base, actual, strconv.Quote(state))
		} else {
			actualPath := append(append([]string{}, s.Path...), s.ValueFrom...)
			actualGuard, actual := guardedPathCEL("o", actualPath)
			value = fmt.Sprintf("%s ? [int(%s == %s ? 1 : 0)] : []", actualGuard, actual, strconv.Quote(state))
		}
		metrics = append(metrics, config.ResourceStateMetricsMetric{Labels: labels, Value: value})
	}
	return metrics
}

func labelsToCEL(labels map[string][]string, base []string, kind config.PathKind) []config.ResourceStateMetricsLabel {
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	slices.Sort(names)
	result := make([]config.ResourceStateMetricsLabel, 0, len(names))
	for _, name := range names {
		// resource-state-metrics injects these labels into every sample.
		if isResourceStateMetricsAutoLabel(name) {
			continue
		}
		relative := labels[name]
		fullPath := append(append([]string{}, base...), relative...)
		guard, resolved := guardedPathCEL("o", fullPath)
		value := fmt.Sprintf("%s ? string(%s) : %s", guard, resolved, strconv.Quote(""))
		if len(base) > 0 && kind == config.PathKindArray {
			value = guardedMapPathCEL(base, relative, false, false)
		}
		result = append(result, config.ResourceStateMetricsLabel{Name: name, Value: value})
	}
	return result
}

func isResourceStateMetricsAutoLabel(name string) bool {
	switch name {
	case "group", "version", "kind", "name", "namespace":
		return true
	default:
		return false
	}
}

func guardedMapPathCEL(base, relative []string, numeric, timestamp bool) string {
	guard, baseExpr := guardedPathCEL("o", base)
	innerGuard, value := guardedPathCEL("v", relative)
	if numeric {
		value = numericPathCEL("v", relative, timestamp)
	} else {
		value = fmt.Sprintf("%s ? string(%s) : %s", innerGuard, value, strconv.Quote(""))
	}
	return fmt.Sprintf("%s ? %s.map(v, %s) : []", guard, baseExpr, value)
}

func mapKeysCEL(path []string) string {
	return fmt.Sprintf("%s.map(k, k)", pathCEL("o", path))
}

func pathCEL(root string, path []string) string {
	_, value := guardedPathCEL(root, path)
	return value
}

var (
	celIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	pathSelector  = regexp.MustCompile(`^\[([^]=]+)=([^]]+)]$`)
)

// guardedPathCEL returns a presence guard and CEL expression for a KSM path.
// KSM's [field=value] list selector is translated to a CEL filter.
func guardedPathCEL(root string, path []string) (string, string) {
	expr := root
	guards := make([]string, 0, len(path))
	for _, part := range path {
		if selector := pathSelector.FindStringSubmatch(part); selector != nil {
			filtered := fmt.Sprintf("%s.filter(x, x[%s] == %s)", expr, strconv.Quote(selector[1]), strconv.Quote(selector[2]))
			guards = append(guards, "size("+filtered+") > 0")
			expr = filtered + "[0]"
			continue
		}

		parent := expr
		if celIdentifier.MatchString(part) {
			expr += "." + part
			guards = append(guards, "has("+expr+")")
		} else {
			expr += "[" + strconv.Quote(part) + "]"
			guards = append(guards, fmt.Sprintf("%s in %s", strconv.Quote(part), parent))
		}
	}
	if len(guards) == 0 {
		return "true", expr
	}
	return strings.Join(guards, " && "), expr
}

func numericPathCEL(root string, path []string, timestamp bool) string {
	value := pathCEL(root, path)
	if timestamp {
		return "unixSeconds(" + value + ")"
	}
	if len(path) > 0 {
		name := strings.ToLower(path[len(path)-1])
		if strings.HasSuffix(name, "time") || strings.HasSuffix(name, "timestamp") {
			return "unixSeconds(" + value + ")"
		}
	}
	return value
}
