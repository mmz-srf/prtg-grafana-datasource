package mockprtg

import (
	"strconv"
	"time"

	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// rootRef is the shared top-level breadcrumb entry every object's Path
// starts from, mirroring PRTG's "Root" group (object id 0).
var rootRef = prtg.ReferencedObject{ID: "0", Name: "Root", Type: "REFERENCED_ROOT"}

// channelSeed declares one channel of a sensorSeed. Exactly one of signal or
// derive is set: signal channels are evaluated directly from (channel key,
// timestamp); derive channels are computed from their sibling channels'
// already-evaluated values (looked up by key), so declaration order within
// a sensorSeed matters -- a derive channel must come after the channels it
// references.
type channelSeed struct {
	name   string
	key    string // reference key for derive lookups within the same sensor
	unit   prtg.Unit
	signal *signalSpec
	derive func(vals map[string]float64) float64
}

type sensorSeed struct {
	name       string
	sensorKind string
	status     string // defaults to "UP" when empty
	channels   []channelSeed
}

type deviceSeed struct {
	name    string
	status  string // defaults to "UP" when empty
	sensors []sensorSeed
}

type groupSeed struct {
	name    string
	devices []deviceSeed
}

type probeSeed struct {
	name   string
	groups []groupSeed
}

// diskCapacityGB is the simulated total capacity backing the "Disk Free"
// sensor's derived free-space-percent channel.
const diskCapacityGB = 512.0

// defaultTopology declares the mock's seed hierarchy: 2 probes, 3 groups, 5
// devices, 11 sensors, spanning ping/CPU/traffic/memory/disk-free sensor
// kinds with realistic units and a few derived (composite) channels.
func defaultTopology() []probeSeed {
	pingChannels := func() []channelSeed {
		return []channelSeed{
			{
				name: "Response Time", key: "latency",
				unit:   prtg.Unit{DisplayUnit: "ms", Type: "custom", DecimalDigits: 0},
				signal: &signalSpec{base: 15, amp1: 8, period1: 45 * time.Minute, amp2: 4, period2: 6 * time.Hour, floor: 1, ceiling: 250},
			},
			{
				name: "Packet Loss", key: "loss",
				unit:   prtg.Unit{DisplayUnit: "%", Type: "percent", DecimalDigits: 1},
				signal: &signalSpec{base: 0.5, amp1: 0.5, period1: 2 * time.Hour, floor: 0, ceiling: 100},
			},
		}
	}
	cpuChannels := func() []channelSeed {
		return []channelSeed{
			{
				name: "Total", key: "total",
				unit:   prtg.Unit{DisplayUnit: "%", Type: "percent", DecimalDigits: 1},
				signal: &signalSpec{base: 35, amp1: 20, period1: 30 * time.Minute, amp2: 10, period2: 4 * time.Hour, floor: 0, ceiling: 100},
			},
			{
				name: "5 Min Avg", key: "avg5",
				unit:   prtg.Unit{DisplayUnit: "%", Type: "percent", DecimalDigits: 1},
				signal: &signalSpec{base: 35, amp1: 12, period1: 90 * time.Minute, floor: 0, ceiling: 100},
			},
		}
	}
	trafficChannels := func() []channelSeed {
		return []channelSeed{
			{
				name: "Traffic In", key: "in",
				unit:   prtg.Unit{DisplayUnit: "Mbit/s", Type: "custom", DecimalDigits: 2},
				signal: &signalSpec{base: 50, amp1: 35, period1: 20 * time.Minute, amp2: 15, period2: 3 * time.Hour, floor: 0, ceiling: 1000},
			},
			{
				name: "Traffic Out", key: "out",
				unit:   prtg.Unit{DisplayUnit: "Mbit/s", Type: "custom", DecimalDigits: 2},
				signal: &signalSpec{base: 30, amp1: 20, period1: 25 * time.Minute, amp2: 10, period2: 3 * time.Hour, floor: 0, ceiling: 1000},
			},
			{
				name: "Traffic Total", key: "total",
				unit:   prtg.Unit{DisplayUnit: "Mbit/s", Type: "custom", DecimalDigits: 2},
				derive: func(vals map[string]float64) float64 { return vals["in"] + vals["out"] },
			},
		}
	}
	memoryChannels := func() []channelSeed {
		return []channelSeed{
			{
				name: "Total", key: "total_gb",
				unit:   prtg.Unit{DisplayUnit: "GB", Type: "custom", DecimalDigits: 1},
				signal: constantSignal(32),
			},
			{
				name: "Used", key: "used_pct",
				unit:   prtg.Unit{DisplayUnit: "%", Type: "percent", DecimalDigits: 1},
				signal: &signalSpec{base: 55, amp1: 20, period1: 40 * time.Minute, floor: 5, ceiling: 95},
			},
			{
				name: "Available", key: "available_gb",
				unit: prtg.Unit{DisplayUnit: "GB", Type: "custom", DecimalDigits: 1},
				derive: func(vals map[string]float64) float64 {
					return vals["total_gb"] * (1 - vals["used_pct"]/100)
				},
			},
		}
	}
	diskChannels := func() []channelSeed {
		return []channelSeed{
			{
				name: "Free Space", key: "free_gb",
				unit:   prtg.Unit{DisplayUnit: "GB", Type: "custom", DecimalDigits: 1},
				signal: &signalSpec{base: 480, amp1: 3, period1: 6 * time.Hour, trendPerDay: -0.15, floor: 20, ceiling: diskCapacityGB},
			},
			{
				name: "Free Space", key: "free_pct",
				unit: prtg.Unit{DisplayUnit: "%", Type: "percent", DecimalDigits: 1},
				derive: func(vals map[string]float64) float64 {
					return vals["free_gb"] / diskCapacityGB * 100
				},
			},
		}
	}

	return []probeSeed{
		{
			name: "Local Probe",
			groups: []groupSeed{
				{
					name: "Core Infrastructure",
					devices: []deviceSeed{
						{
							name: "core-router-01",
							sensors: []sensorSeed{
								{name: "Ping", sensorKind: "ping", channels: pingChannels()},
								{name: "CPU Load", sensorKind: "cpu", channels: cpuChannels()},
							},
						},
						{
							name: "core-switch-01",
							sensors: []sensorSeed{
								{name: "Traffic", sensorKind: "traffic", channels: trafficChannels()},
								{name: "Ping", sensorKind: "ping", channels: pingChannels()},
							},
						},
					},
				},
				{
					name: "Application Servers",
					devices: []deviceSeed{
						{
							name: "app-server-01",
							sensors: []sensorSeed{
								{name: "CPU Load", sensorKind: "cpu", channels: cpuChannels()},
								{name: "Memory", sensorKind: "memory", channels: memoryChannels()},
								{name: "Disk Free (C:)", sensorKind: "diskfree", channels: diskChannels()},
							},
						},
						{
							name: "app-server-02", status: "WARNING",
							sensors: []sensorSeed{
								{name: "Ping", sensorKind: "ping", status: "WARNING", channels: pingChannels()},
								{name: "CPU Load", sensorKind: "cpu", channels: cpuChannels()},
							},
						},
					},
				},
			},
		},
		{
			name: "Branch Office Probe",
			groups: []groupSeed{
				{
					name: "Branch Network",
					devices: []deviceSeed{
						{
							name: "branch-switch-01",
							sensors: []sensorSeed{
								{name: "Traffic", sensorKind: "traffic", channels: trafficChannels()},
								{name: "Ping", sensorKind: "ping", channels: pingChannels()},
							},
						},
					},
				},
			},
		},
	}
}

// channelRuntime is the resolved, request-time evaluation spec for one
// channel, keyed by its assigned object id.
type channelRuntime struct {
	id     string
	key    string
	signal *signalSpec
	derive func(vals map[string]float64) float64
}

// Dataset is a fully-built, immutable-after-construction mock PRTG object
// hierarchy: the exact prtg package types the real Client expects to decode,
// plus the parent/ancestor lookups and per-channel evaluation specs the mock
// server's routes need at request time.
type Dataset struct {
	Probes           []prtg.GroupInfo
	Groups           []prtg.GroupInfo
	Devices          []prtg.DeviceInfo
	Sensors          []prtg.SensorInfo
	ChannelsBySensor map[string][]prtg.ChannelInfo

	// parentOf maps an object's id to its direct parent's id (a group/probe's
	// parent, a device's owning group/probe, a sensor's owning device).
	parentOf map[string]string
	// ancestorsOf maps an object's id to every ancestor id from the root
	// down (excluding itself), for `ancestors.id` filter matching.
	ancestorsOf map[string][]string
	// channelRuntimeBySensor holds each sensor's channels' evaluation specs,
	// in declaration order (derive channels rely on that order to see their
	// dependencies already evaluated).
	channelRuntimeBySensor map[string][]channelRuntime
}

// DefaultDataset builds the mock's seed dataset from defaultTopology.
func DefaultDataset() *Dataset {
	return buildDataset(defaultTopology())
}

func buildDataset(topology []probeSeed) *Dataset {
	ds := &Dataset{
		ChannelsBySensor:       map[string][]prtg.ChannelInfo{},
		parentOf:               map[string]string{},
		ancestorsOf:            map[string][]string{},
		channelRuntimeBySensor: map[string][]channelRuntime{},
	}

	nextID := 2001
	newID := func() string {
		id := strconv.Itoa(nextID)
		nextID++
		return id
	}
	statusOr := func(s string) string {
		if s == "" {
			return "UP"
		}
		return s
	}
	appendPath := func(path []prtg.ReferencedObject, ref prtg.ReferencedObject) []prtg.ReferencedObject {
		out := make([]prtg.ReferencedObject, len(path)+1)
		copy(out, path)
		out[len(path)] = ref
		return out
	}
	appendAncestors := func(ancestors []string, id string) []string {
		out := make([]string, len(ancestors)+1)
		copy(out, ancestors)
		out[len(ancestors)] = id
		return out
	}

	for _, ps := range topology {
		probeID := newID()
		probeRef := prtg.ReferencedObject{ID: probeID, Name: ps.name, Type: "REFERENCED_PROBE"}
		probePath := appendPath([]prtg.ReferencedObject{rootRef}, probeRef)
		ds.Probes = append(ds.Probes, prtg.GroupInfo{
			ID: probeID, Name: ps.name, Status: "UP", Path: probePath,
			Priority: 3, ScanningInterval: "60s",
		})
		ds.parentOf[probeID] = rootRef.ID
		ds.ancestorsOf[probeID] = []string{rootRef.ID}

		for _, gs := range ps.groups {
			groupID := newID()
			groupRef := prtg.ReferencedObject{ID: groupID, Name: gs.name, Type: "REFERENCED_GROUP"}
			groupPath := appendPath(probePath, groupRef)
			ds.Groups = append(ds.Groups, prtg.GroupInfo{
				ID: groupID, Name: gs.name, Status: "UP", Path: groupPath,
				Priority: 3, ScanningInterval: "60s",
			})
			ds.parentOf[groupID] = probeID
			ds.ancestorsOf[groupID] = appendAncestors(ds.ancestorsOf[probeID], probeID)

			for _, dvs := range gs.devices {
				deviceID := newID()
				deviceRef := prtg.ReferencedObject{ID: deviceID, Name: dvs.name, Type: "REFERENCED_DEVICE"}
				devicePath := appendPath(groupPath, deviceRef)
				ds.Devices = append(ds.Devices, prtg.DeviceInfo{
					ID: deviceID, Name: dvs.name, Status: statusOr(dvs.status), Path: devicePath,
					Priority: 3, ScanningInterval: "60s",
				})
				ds.parentOf[deviceID] = groupID
				ds.ancestorsOf[deviceID] = appendAncestors(ds.ancestorsOf[groupID], groupID)

				for _, sns := range dvs.sensors {
					sensorID := newID()
					sensorRef := prtg.ReferencedObject{ID: sensorID, Name: sns.name, Type: "REFERENCED_SENSOR"}
					sensorPath := appendPath(devicePath, sensorRef)

					channelRefs := make([]prtg.ReferencedObject, 0, len(sns.channels))
					channelInfos := make([]prtg.ChannelInfo, 0, len(sns.channels))
					runtimes := make([]channelRuntime, 0, len(sns.channels))
					for i, cs := range sns.channels {
						chID := strconv.Itoa(i)
						chRef := prtg.ReferencedObject{ID: chID, Name: cs.name, Type: "REFERENCED_CHANNEL"}
						chPath := appendPath(sensorPath, chRef)

						channelRefs = append(channelRefs, chRef)
						channelInfos = append(channelInfos, prtg.ChannelInfo{
							ID: chID, Name: cs.name, InternalName: cs.key,
							Unit: cs.unit, Path: chPath,
						})
						runtimes = append(runtimes, channelRuntime{
							id: chID, key: cs.key, signal: cs.signal, derive: cs.derive,
						})
					}

					ds.Sensors = append(ds.Sensors, prtg.SensorInfo{
						ID: sensorID, Name: sns.name, Status: statusOr(sns.status), Path: sensorPath,
						Priority: 3, ScanningInterval: "60s", Channels: channelRefs,
						SensorKind: sns.sensorKind, StatusSince: epoch.Format(time.RFC3339),
					})
					ds.ChannelsBySensor[sensorID] = channelInfos
					ds.channelRuntimeBySensor[sensorID] = runtimes
					ds.parentOf[sensorID] = deviceID
					ds.ancestorsOf[sensorID] = appendAncestors(ds.ancestorsOf[deviceID], deviceID)
				}
			}
		}
	}

	return ds
}

// EvaluateSensor returns every channel of sensorID's value at time t, keyed
// by channel id (e.g. {"0": 42.1, "1": 98.6}). Channels are evaluated in
// declaration order so a derive channel can see its dependencies' values.
func (ds *Dataset) EvaluateSensor(sensorID string, t time.Time) map[string]float64 {
	runtimes := ds.channelRuntimeBySensor[sensorID]
	if len(runtimes) == 0 {
		return nil
	}

	byKey := make(map[string]float64, len(runtimes))
	byID := make(map[string]float64, len(runtimes))
	for _, rt := range runtimes {
		var v float64
		switch {
		case rt.signal != nil:
			v = rt.signal.evaluate(sensorID+"."+rt.id, t)
		case rt.derive != nil:
			v = rt.derive(byKey)
		}
		byKey[rt.key] = v
		byID[rt.id] = v
	}
	return byID
}

// sampleChannelOverWindow evaluates a single channel at n evenly-spaced
// points across [t-span, t], returning its average/min/max -- used to give
// ChannelInfo.LastMeasurement's Average/Minimum/Maximum a touch of realism
// instead of leaving them equal to the instantaneous value.
func (ds *Dataset) sampleChannelOverWindow(sensorID, chID string, t time.Time, span time.Duration, n int) (avg, min, max float64, ok bool) {
	if n <= 0 {
		return 0, 0, 0, false
	}
	step := span / time.Duration(n)
	var sum float64
	count := 0
	for i := 0; i < n; i++ {
		sampleT := t.Add(-span + time.Duration(i)*step)
		v, exists := ds.EvaluateSensor(sensorID, sampleT)[chID]
		if !exists {
			continue
		}
		if count == 0 || v < min {
			min = v
		}
		if count == 0 || v > max {
			max = v
		}
		sum += v
		count++
	}
	if count == 0 {
		return 0, 0, 0, false
	}
	return sum / float64(count), min, max, true
}
