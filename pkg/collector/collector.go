/*
Copyright 2021.

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

package collector

import (
	"fmt"
	"strconv"

	"github.com/sustainable-computing-io/kepler/pkg/attacher"

	"github.com/prometheus/client_golang/prometheus"
)

type Collector struct {
	modules *attacher.BpfModuleTables
}

func New() (*Collector, error) {
	return &Collector{}, nil
}

func (c *Collector) Attach() error {
	m, err := attacher.AttachBPFAssets()
	if err != nil {
		return fmt.Errorf("failed to attach bpf assets: %v", err)
	}
	c.modules = m
	c.reader()
	return nil
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	lock.Lock()
	defer lock.Unlock()
	for k, _ := range podEnergy {
		desc := prometheus.NewDesc(
			"pod_stat_"+k,
			"Pod energy consumption status",
			[]string{
				"pod_name",
				"pod_namespace",
				"command",
				"last_cpu_time",
				"curr_cpu_time",
				"last_cpu_cycles",
				"curr_cpu_cycles",
				"last_cpu_instructions",
				"curr_cpu_instructions",
				"last_cache_misses",
				"curr_cache_misses",
				"last_energy_in_core",
				"curr_energy_in_core",
				"last_energy_in_dram",
				"curr_energy_in_dram",
			},
			nil,
		)
		ch <- desc
	}
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	lock.Lock()
	defer lock.Unlock()
	for _, v := range podEnergy {
		de := prometheus.NewDesc(
			"pod_energy_total",
			"Pod total energy consumption",
			[]string{
				"pod_name",
				"pod_namespace",
				"command",
				"last_cpu_time",
				"curr_cpu_time",
				"last_cpu_cycles",
				"curr_cpu_cycles",
				"last_cpu_instructions",
				"curr_cpu_instructions",
				"last_cache_misses",
				"curr_cache_misses",
				"last_energy_in_core",
				"curr_energy_in_core",
				"last_energy_in_dram",
				"curr_energy_in_dram",
			},
			nil,
		)
		desc := prometheus.MustNewConstMetric(
			de,
			prometheus.CounterValue,
			float64(v.LastEnergyInCore+v.LastEnergyInDram),
			v.Pod, v.Namespace, v.Command,
			strconv.FormatUint(v.LastCPUTime, 10), strconv.FormatUint(v.CPUTime, 10),
			strconv.FormatUint(v.LastCPUCycles, 10), strconv.FormatUint(v.CPUCycles, 10),
			strconv.FormatUint(v.LastCPUInstr, 10), strconv.FormatUint(v.CPUInstr, 10),
			strconv.FormatUint(v.LastCacheMisses, 10), strconv.FormatUint(v.CacheMisses, 10),
			strconv.FormatUint(v.LastEnergyInCore, 10), strconv.FormatUint(v.EnergyInCore, 10),
			strconv.FormatUint(v.LastEnergyInDram, 10), strconv.FormatUint(v.EnergyInDram, 10),
		)
		ch <- desc

		de_current := prometheus.NewDesc(
			"pod_energy_current",
			"Pod current energy consumption",
			[]string{
				"pod_name",
				"pod_namespace",
				"command",
			},
			nil,
		)
		desc_current := prometheus.MustNewConstMetric(
			de_current,
			prometheus.CounterValue,
			float64(v.EnergyInCore+v.EnergyInDram),
			v.Pod, v.Namespace, v.Command,
		)
		ch <- desc_current
	}
}

func (c *Collector) Destroy() {
	if c.modules != nil {
		attacher.DetachBPFModules(c.modules)
	}
}
