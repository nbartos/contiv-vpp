//  Copyright (c) 2018 Cisco and/or its affiliates.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at:
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package telemetry

import (
	"strconv"
	"time"

	govppapi "git.fd.io/govpp.git/api"
	"github.com/ligato/cn-infra/flavors/local"
	prom "github.com/ligato/cn-infra/rpc/prometheus"
	"github.com/ligato/cn-infra/utils/safeclose"
	"github.com/ligato/vpp-agent/plugins/govppmux"
	"github.com/ligato/vpp-agent/plugins/govppmux/vppcalls"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// Period between metric updates
	updatePeriod = time.Second * 5

	// Registry path for telemetry metrics
	registryPath = "/vpp"

	// Metrics label used for agent label
	agentLabel = "agent"
)

const (
	// Runtime
	runtimeThreadLabel   = "thread"
	runtimeThreadIDLabel = "threadID"
	runtimeItemLabel     = "item"

	runtimeCallsMetric          = "calls"
	runtimeVectorsMetric        = "vectors"
	runtimeSuspendsMetric       = "suspends"
	runtimeClocksMetric         = "clocks"
	runtimeVectorsPerCallMetric = "vectors_per_call"

	// Memory
	memoryThreadLabel   = "thread"
	memoryThreadIDLabel = "threadID"

	memoryObjectsMetric   = "objects"
	memoryUsedMetric      = "used"
	memoryTotalMetric     = "total"
	memoryFreeMetric      = "free"
	memoryReclaimedMetric = "reclaimed"
	memoryOverheadMetric  = "overhead"
	memoryCapacityMetric  = "capacity"

	// Buffers
	buffersThreadIDLabel = "threadID"
	buffersItemLabel     = "item"
	buffersIndexLabel    = "index"

	buffersSizeMetric     = "size"
	buffersAllocMetric    = "alloc"
	buffersFreeMetric     = "free"
	buffersNumAllocMetric = "num_alloc"
	buffersNumFreeMetric  = "num_free"

	// Node counters
	nodeCounterItemLabel   = "item"
	nodeCounterReasonLabel = "reason"

	nodeCounterCountMetric = "count"
)

// Plugin registers Telemetry Plugin
type Plugin struct {
	Deps

	vppCh govppapi.Channel

	runtimeGaugeVecs map[string]*prometheus.GaugeVec
	runtimeStats     map[string]*runtimeStats

	memoryGaugeVecs map[string]*prometheus.GaugeVec
	memoryStats     map[string]*memoryStats

	buffersGaugeVecs map[string]*prometheus.GaugeVec
	buffersStats     map[string]*buffersStats

	nodeCounterGaugeVecs map[string]*prometheus.GaugeVec
	nodeCounterStats     map[string]*nodeCounterStats
}

// Deps represents dependencies of Telemetry Plugin
type Deps struct {
	local.PluginInfraDeps

	GoVppmux   govppmux.API
	Prometheus prom.API
}

type runtimeStats struct {
	threadName string
	threadID   uint
	itemName   string
	metrics    map[string]prometheus.Gauge
}

type memoryStats struct {
	threadName string
	threadID   uint
	metrics    map[string]prometheus.Gauge
}

type buffersStats struct {
	threadID  uint
	itemName  string
	itemIndex uint
	metrics   map[string]prometheus.Gauge
}

type nodeCounterStats struct {
	itemName string
	metrics  map[string]prometheus.Gauge
}

// Init initializes Telemetry Plugin
func (p *Plugin) Init() error {
	// Register '/vpp' registry path
	err := p.Prometheus.NewRegistry(registryPath, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
	if err != nil {
		return err
	}

	// Runtime metrics
	p.runtimeGaugeVecs = make(map[string]*prometheus.GaugeVec)
	p.runtimeStats = make(map[string]*runtimeStats)

	for _, metric := range [][2]string{
		{runtimeCallsMetric, "Number of calls"},
		{runtimeVectorsMetric, "Number of vectors"},
		{runtimeSuspendsMetric, "Number of suspends"},
		{runtimeClocksMetric, "Number of clocks"},
		{runtimeVectorsPerCallMetric, "Number of vectors per call"},
	} {
		name := metric[0]
		p.runtimeGaugeVecs[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "vpp",
			Subsystem: "runtime",
			Name:      name,
			Help:      metric[1],
			ConstLabels: prometheus.Labels{
				agentLabel: p.ServiceLabel.GetAgentLabel(),
			},
		}, []string{runtimeItemLabel, runtimeThreadLabel, runtimeThreadIDLabel})

	}

	// register created vectors to prometheus
	for name, metric := range p.runtimeGaugeVecs {
		if err := p.Prometheus.Register(registryPath, metric); err != nil {
			p.Log.Errorf("failed to register %v metric: %v", name, err)
			return err
		}
	}

	// Memory metrics
	p.memoryGaugeVecs = make(map[string]*prometheus.GaugeVec)
	p.memoryStats = make(map[string]*memoryStats)

	for _, metric := range [][2]string{
		{memoryObjectsMetric, "Number of objects"},
		{memoryUsedMetric, "Used memory"},
		{memoryTotalMetric, "Total memory"},
		{memoryFreeMetric, "Free memory"},
		{memoryReclaimedMetric, "Reclaimed memory"},
		{memoryOverheadMetric, "Overhead"},
		{memoryCapacityMetric, "Capacity"},
	} {
		name := metric[0]
		p.memoryGaugeVecs[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "vpp",
			Subsystem: "memory",
			Name:      name,
			Help:      metric[1],
			ConstLabels: prometheus.Labels{
				agentLabel: p.ServiceLabel.GetAgentLabel(),
			},
		}, []string{memoryThreadLabel, memoryThreadIDLabel})

	}

	// register created vectors to prometheus
	for name, metric := range p.memoryGaugeVecs {
		if err := p.Prometheus.Register(registryPath, metric); err != nil {
			p.Log.Errorf("failed to register %v metric: %v", name, err)
			return err
		}
	}

	// Buffers metrics
	p.buffersGaugeVecs = make(map[string]*prometheus.GaugeVec)
	p.buffersStats = make(map[string]*buffersStats)

	for _, metric := range [][2]string{
		{buffersSizeMetric, "Size of buffer"},
		{buffersAllocMetric, "Allocated"},
		{buffersFreeMetric, "Free"},
		{buffersNumAllocMetric, "Number of allocated"},
		{buffersNumFreeMetric, "Number of free"},
	} {
		name := metric[0]
		p.buffersGaugeVecs[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "vpp",
			Subsystem: "buffers",
			Name:      name,
			Help:      metric[1],
			ConstLabels: prometheus.Labels{
				agentLabel: p.ServiceLabel.GetAgentLabel(),
			},
		}, []string{buffersThreadIDLabel, buffersItemLabel, buffersIndexLabel})

	}

	// register created vectors to prometheus
	for name, metric := range p.buffersGaugeVecs {
		if err := p.Prometheus.Register(registryPath, metric); err != nil {
			p.Log.Errorf("failed to register %v metric: %v", name, err)
			return err
		}
	}

	// Node counters metrics
	p.nodeCounterGaugeVecs = make(map[string]*prometheus.GaugeVec)
	p.nodeCounterStats = make(map[string]*nodeCounterStats)

	for _, metric := range [][2]string{
		{nodeCounterCountMetric, "Count"},
	} {
		name := metric[0]
		p.nodeCounterGaugeVecs[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "vpp",
			Subsystem: "node_counter",
			Name:      name,
			Help:      metric[1],
			ConstLabels: prometheus.Labels{
				agentLabel: p.ServiceLabel.GetAgentLabel(),
			},
		}, []string{nodeCounterItemLabel, nodeCounterReasonLabel})

	}

	// register created vectors to prometheus
	for name, metric := range p.nodeCounterGaugeVecs {
		if err := p.Prometheus.Register(registryPath, metric); err != nil {
			p.Log.Errorf("failed to register %v metric: %v", name, err)
			return err
		}
	}

	// Create GoVPP channel
	p.vppCh, err = p.GoVppmux.NewAPIChannel()
	if err != nil {
		p.Log.Errorf("Error creating channel: %v", err)
		return err
	}

	return nil
}

// AfterInit executes after initializion of Telemetry Plugin
func (p *Plugin) AfterInit() error {
	// Periodically update data
	go func() {
		for {
			// Update runtime
			runtimeInfo, err := vppcalls.GetRuntimeInfo(p.vppCh)
			if err != nil {
				p.Log.Errorf("Command failed: %v", err)
			} else {
				for _, thread := range runtimeInfo.Threads {
					for _, item := range thread.Items {
						stats, ok := p.runtimeStats[item.Name]
						if !ok {
							stats = &runtimeStats{
								threadID:   thread.ID,
								threadName: thread.Name,
								itemName:   item.Name,
								metrics:    map[string]prometheus.Gauge{},
							}

							// add gauges with corresponding labels into vectors
							for k, vec := range p.runtimeGaugeVecs {
								stats.metrics[k], err = vec.GetMetricWith(prometheus.Labels{
									runtimeItemLabel:     item.Name,
									runtimeThreadLabel:   thread.Name,
									runtimeThreadIDLabel: strconv.Itoa(int(thread.ID)),
								})
								if err != nil {
									p.Log.Error(err)
								}
							}
						}

						stats.metrics[runtimeCallsMetric].Set(float64(item.Calls))
						stats.metrics[runtimeVectorsMetric].Set(float64(item.Vectors))
						stats.metrics[runtimeSuspendsMetric].Set(float64(item.Suspends))
						stats.metrics[runtimeClocksMetric].Set(item.Clocks)
						stats.metrics[runtimeVectorsPerCallMetric].Set(item.VectorsPerCall)
					}
				}
			}

			// Update memory
			memoryInfo, err := vppcalls.GetMemory(p.vppCh)
			if err != nil {
				p.Log.Errorf("Command failed: %v", err)
			} else {
				for _, thread := range memoryInfo.Threads {
					stats, ok := p.memoryStats[thread.Name]
					if !ok {
						stats = &memoryStats{
							threadName: thread.Name,
							threadID:   thread.ID,
							metrics:    map[string]prometheus.Gauge{},
						}

						// add gauges with corresponding labels into vectors
						for k, vec := range p.memoryGaugeVecs {
							stats.metrics[k], err = vec.GetMetricWith(prometheus.Labels{
								memoryThreadLabel:   thread.Name,
								memoryThreadIDLabel: strconv.Itoa(int(thread.ID)),
							})
							if err != nil {
								p.Log.Error(err)
							}
						}
					}

					stats.metrics[memoryObjectsMetric].Set(float64(thread.Objects))
					stats.metrics[memoryUsedMetric].Set(float64(thread.Used))
					stats.metrics[memoryTotalMetric].Set(float64(thread.Total))
					stats.metrics[memoryFreeMetric].Set(float64(thread.Free))
					stats.metrics[memoryReclaimedMetric].Set(float64(thread.Reclaimed))
					stats.metrics[memoryOverheadMetric].Set(float64(thread.Overhead))
					stats.metrics[memoryCapacityMetric].Set(float64(thread.Capacity))
				}
			}

			// Update buffers
			buffersInfo, err := vppcalls.GetBuffersInfo(p.vppCh)
			if err != nil {
				p.Log.Errorf("Command failed: %v", err)
			} else {
				for _, item := range buffersInfo.Items {
					stats, ok := p.buffersStats[item.Name]
					if !ok {
						stats = &buffersStats{
							threadID:  item.ThreadID,
							itemName:  item.Name,
							itemIndex: item.Index,
							metrics:   map[string]prometheus.Gauge{},
						}

						// add gauges with corresponding labels into vectors
						for k, vec := range p.buffersGaugeVecs {
							stats.metrics[k], err = vec.GetMetricWith(prometheus.Labels{
								buffersThreadIDLabel: strconv.Itoa(int(item.ThreadID)),
								buffersItemLabel:     item.Name,
								buffersIndexLabel:    strconv.Itoa(int(item.Index)),
							})
							if err != nil {
								p.Log.Error(err)
							}
						}
					}

					stats.metrics[buffersSizeMetric].Set(float64(item.Size))
					stats.metrics[buffersAllocMetric].Set(float64(item.Alloc))
					stats.metrics[buffersFreeMetric].Set(float64(item.Free))
					stats.metrics[buffersNumAllocMetric].Set(float64(item.NumAlloc))
					stats.metrics[buffersNumFreeMetric].Set(float64(item.NumFree))
				}
			}

			// Update node counters
			nodeCountersInfo, err := vppcalls.GetNodeCounters(p.vppCh)
			if err != nil {
				p.Log.Errorf("Command failed: %v", err)
			} else {
				for _, item := range nodeCountersInfo.Counters {
					stats, ok := p.nodeCounterStats[item.Node]
					if !ok {
						stats = &nodeCounterStats{
							itemName: item.Node,
							metrics:  map[string]prometheus.Gauge{},
						}

						// add gauges with corresponding labels into vectors
						for k, vec := range p.nodeCounterGaugeVecs {
							stats.metrics[k], err = vec.GetMetricWith(prometheus.Labels{
								nodeCounterItemLabel:   item.Node,
								nodeCounterReasonLabel: item.Reason,
							})
							if err != nil {
								p.Log.Error(err)
							}
						}
					}

					stats.metrics[nodeCounterCountMetric].Set(float64(item.Count))
				}
			}

			// Delay period between updates
			time.Sleep(updatePeriod)
		}
	}()
	return nil
}

// Close is used to clean up resources used by Telemetry Plugin
func (p *Plugin) Close() error {
	return safeclose.Close(p.vppCh)
}
