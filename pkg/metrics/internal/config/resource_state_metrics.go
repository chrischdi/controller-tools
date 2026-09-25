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

package config

// ResourceStateMetricsVersion is the resource-state-metrics version whose API
// is represented by the types below.
const ResourceStateMetricsVersion = "v0.1.0"

// ResourceMetricsMonitor is a resource-state-metrics configuration resource.
type ResourceMetricsMonitor struct {
	APIVersion string                     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                     `yaml:"kind" json:"kind"`
	Metadata   ResourceMetricsMonitorMeta `yaml:"metadata" json:"metadata"`
	Spec       ResourceMetricsMonitorSpec `yaml:"spec" json:"spec"`
}

type ResourceMetricsMonitorMeta struct {
	Name      string `yaml:"name" json:"name"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
}

type ResourceMetricsMonitorSpec struct {
	Configuration ResourceStateMetricsConfiguration `yaml:"configuration" json:"configuration"`
}

type ResourceStateMetricsConfiguration struct {
	Stores []ResourceStateMetricsStore `yaml:"stores" json:"stores"`
}

type ResourceStateMetricsStore struct {
	Group    string                       `yaml:"group" json:"group"`
	Version  string                       `yaml:"version" json:"version"`
	Kind     string                       `yaml:"kind" json:"kind"`
	Resource string                       `yaml:"resource" json:"resource"`
	Resolver string                       `yaml:"resolver" json:"resolver"`
	Labels   []ResourceStateMetricsLabel  `yaml:"labels,omitempty" json:"labels,omitempty"`
	Families []ResourceStateMetricsFamily `yaml:"families" json:"families"`
}

type ResourceStateMetricsFamily struct {
	Name    string                       `yaml:"name" json:"name"`
	Help    string                       `yaml:"help" json:"help"`
	Metrics []ResourceStateMetricsMetric `yaml:"metrics" json:"metrics"`
}

type ResourceStateMetricsMetric struct {
	Labels []ResourceStateMetricsLabel `yaml:"labels,omitempty" json:"labels,omitempty"`
	Value  string                      `yaml:"value" json:"value"`
}

type ResourceStateMetricsLabel struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"`
}
