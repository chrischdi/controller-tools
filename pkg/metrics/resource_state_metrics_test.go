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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"sigs.k8s.io/controller-tools/pkg/metrics/internal/config"
)

func TestToResourceStateMetrics(t *testing.T) {
	prefix := "foo"
	resource := config.Resource{
		MetricNamePrefix: &prefix,
		GroupVersionKind: config.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Policy"},
		Labels: config.Labels{LabelsFromPath: map[string][]string{
			"cluster": {"metadata", "labels", "cluster"},
			"name":    {"metadata", "name"},
		}},
		Metrics: []config.Generator{
			{Name: "condition", Help: "A condition.", Each: config.Metric{Type: config.MetricTypeStateSet, StateSet: &config.MetricStateSet{
				MetricMeta: config.MetricMeta{Path: []string{"status", "conditions"}, PathKind: config.PathKindArray, LabelsFromPath: map[string][]string{"type": {"type"}}},
				List:       []string{"True", "False"}, LabelName: "status", ValueFrom: []string{"status"},
			}}},
			{Name: "transition", Help: "Transition time.", Each: config.Metric{Type: config.MetricTypeGauge, Gauge: &config.MetricGauge{
				MetricMeta: config.MetricMeta{Path: []string{"status", "conditions"}, PathKind: config.PathKindArray}, ValueFrom: []string{"lastTransitionTime"},
			}}},
		},
	}

	got := toResourceStateMetrics(resource)
	if got.Resource != "policies" || got.Resolver != "cel" {
		t.Fatalf("unexpected store: %#v", got)
	}
	if len(got.Labels) != 0 {
		t.Errorf("expected store labels to be rendered on metrics, got %#v", got.Labels)
	}
	metricLabels := got.Families[0].Metrics[0].Labels
	if diff := cmp.Diff(`has(o.metadata) && has(o.metadata.labels) && has(o.metadata.labels.cluster) ? string(o.metadata.labels.cluster) : ""`, metricLabels[2].Value); diff != "" {
		t.Errorf("store label expression (-want,+got): %s", diff)
	}
	if len(metricLabels) != 3 {
		t.Errorf("expected the automatically injected name label to be omitted, got %#v", metricLabels)
	}
	if diff := cmp.Diff(`has(o.status) && has(o.status.conditions) ? o.status.conditions.map(v, int(v.status == "True" ? 1 : 0)) : []`, got.Families[0].Metrics[0].Value); diff != "" {
		t.Errorf("state set value expression (-want,+got): %s", diff)
	}
	if diff := cmp.Diff(`has(o.status) && has(o.status.conditions) ? o.status.conditions.map(v, unixSeconds(v.lastTransitionTime)) : []`, got.Families[1].Metrics[0].Value); diff != "" {
		t.Errorf("timestamp value expression (-want,+got): %s", diff)
	}
}

func TestResourceStateMetricsScalarAndObjectCEL(t *testing.T) {
	stateSet := stateSetToCEL(config.MetricStateSet{
		MetricMeta: config.MetricMeta{Path: []string{"status", "phase"}, PathKind: config.PathKindScalar},
		List:       []string{"Ready"}, LabelName: "phase",
	})
	if diff := cmp.Diff(`has(o.status) && has(o.status.phase) ? [int(o.status.phase == "Ready" ? 1 : 0)] : []`, stateSet[0].Value); diff != "" {
		t.Errorf("scalar state set expression (-want,+got): %s", diff)
	}

	info := infoToCEL(config.MetricInfo{
		MetricMeta: config.MetricMeta{
			Path:           []string{"status", "nodeRef"},
			PathKind:       config.PathKindObject,
			LabelsFromPath: map[string][]string{"node_name": {"name"}},
		},
	})
	if diff := cmp.Diff(`has(o.status) && has(o.status.nodeRef) && has(o.status.nodeRef.name) ? string(o.status.nodeRef.name) : ""`, info.Labels[0].Value); diff != "" {
		t.Errorf("object label expression (-want,+got): %s", diff)
	}
	if diff := cmp.Diff(`has(o.status) && has(o.status.nodeRef) ? [1] : []`, info.Value); diff != "" {
		t.Errorf("object value expression (-want,+got): %s", diff)
	}

	gauge := gaugeToCEL(config.MetricGauge{
		MetricMeta: config.MetricMeta{Path: []string{"spec", "paused"}, PathKind: config.PathKindScalar},
		NilIsZero:  true,
	})
	if diff := cmp.Diff(`has(o.spec) && has(o.spec.paused) ? o.spec.paused : 0`, gauge.Value); diff != "" {
		t.Errorf("nil-is-zero expression (-want,+got): %s", diff)
	}
}

func TestResourceStateMetricsSelectorCEL(t *testing.T) {
	guard, value := guardedPathCEL("o", []string{"metadata", "ownerReferences", "[kind=Cluster]", "name"})
	if !strings.Contains(guard, `size(o.metadata.ownerReferences.filter(x, x["kind"] == "Cluster")) > 0`) {
		t.Errorf("selector guard does not check for a matching element: %s", guard)
	}
	if diff := cmp.Diff(`o.metadata.ownerReferences.filter(x, x["kind"] == "Cluster")[0].name`, value); diff != "" {
		t.Errorf("selector value expression (-want,+got): %s", diff)
	}
}
