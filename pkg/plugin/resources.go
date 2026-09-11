package plugin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// maxPickerItems bounds the plain hierarchy-browsing routes (groups,
// devices, sensors, channels). These routes return a bare array per the
// unified contract (no truncated flag), so the cap is set generously (at
// PRTG APIv2's own documented per-request maximum) to make silent
// truncation practically a non-issue for a picker dropdown.
const maxPickerItems = 3000

// idNameDTO is the small, stable shape returned for devices/sensors: PRTG's
// "Experimental" schemas may shift across releases, but this contract
// won't.
type idNameDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// groupDTO additionally carries a breadcrumb Path: the "groups" route
// flattens groups and probes at every depth into one list (see plan §5
// open item (a)), so identically-named nested groups need something to
// disambiguate them in a picker.
type groupDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// channelDTO additionally carries the channel's display unit, which is
// useful (but optional/additive) context for a channel picker.
type channelDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit,omitempty"`
}

type channelMatchDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type sensorMatchDTO struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	DeviceID        string            `json:"deviceId"`
	DeviceName      string            `json:"deviceName"`
	MatchedChannels []channelMatchDTO `json:"matchedChannels"`
}

type sensorSearchResponseDTO struct {
	Sensors       []sensorMatchDTO `json:"sensors"`
	TotalSensors  int              `json:"totalSensors"`
	TotalChannels int              `json:"totalChannels"`
	Truncated     bool             `json:"truncated"`
}

// registerRoutes builds the http.ServeMux implementing the 5 CallResource
// routes of the unified contract, using Go 1.22+'s method+pattern mux
// matching.
func (d *Datasource) registerRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /groups", d.handleGroups)
	mux.HandleFunc("GET /devices", d.handleDevices)
	mux.HandleFunc("GET /sensors", d.handleSensors)
	mux.HandleFunc("GET /channels", d.handleChannels)
	mux.HandleFunc("GET /sensors/search", d.handleSensorsSearch)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErrorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeUpstreamError maps an error from the prtg package to an HTTP
// response, preferring a parsed *prtg.APIError's status/code/message over a
// generic 500.
func writeUpstreamError(w http.ResponseWriter, err error) {
	if apiErr, ok := prtg.AsAPIError(err); ok {
		status := apiErr.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusInternalServerError
		}
		message := apiErr.Message
		if apiErr.Code != "" {
			message = apiErr.Code + ": " + apiErr.Message
		}
		writeErrorJSON(w, status, message)
		return
	}
	writeErrorJSON(w, http.StatusInternalServerError, err.Error())
}

// pathBreadcrumb renders a ReferencedObject path as a "A / B / C" string for
// display purposes.
func pathBreadcrumb(path []prtg.ReferencedObject) string {
	if len(path) == 0 {
		return ""
	}
	names := make([]string, 0, len(path))
	for _, ref := range path {
		if ref.Name != "" {
			names = append(names, ref.Name)
		}
	}
	return strings.Join(names, " / ")
}

// handleGroups implements GET /groups: the top "group tier" for the
// hierarchy picker, combining PRTG's separate /experimental/groups and
// /experimental/probes endpoints into one flat, indistinguishable list (see
// plan §5 open item (a) -- verify this UX choice against a live server).
func (d *Datasource) handleGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	groups, err := prtg.FetchAll[prtg.GroupInfo](ctx, d.client, "/experimental/groups", prtg.FetchAllOptions{
		Include:  "path",
		MaxItems: maxPickerItems,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	probes, err := prtg.FetchAll[prtg.GroupInfo](ctx, d.client, "/experimental/probes", prtg.FetchAllOptions{
		Include:  "path",
		MaxItems: maxPickerItems,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	items := make([]groupDTO, 0, len(groups.Items)+len(probes.Items))
	for _, g := range groups.Items {
		items = append(items, groupDTO{ID: g.ID, Name: g.Name, Path: pathBreadcrumb(g.Path)})
	}
	for _, p := range probes.Items {
		items = append(items, groupDTO{ID: p.ID, Name: p.Name, Path: pathBreadcrumb(p.Path)})
	}
	writeJSON(w, http.StatusOK, items)
}

// handleDevices implements GET /devices?groupId=<id>: the direct child
// devices of a given group/probe.
func (d *Datasource) handleDevices(w http.ResponseWriter, r *http.Request) {
	groupID := r.URL.Query().Get("groupId")
	if groupID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "groupId is required")
		return
	}

	devices, err := prtg.FetchAll[prtg.DeviceInfo](r.Context(), d.client, "/experimental/devices", prtg.FetchAllOptions{
		Filter:   prtg.ParentID(groupID).String(),
		MaxItems: maxPickerItems,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	items := make([]idNameDTO, 0, len(devices.Items))
	for _, dv := range devices.Items {
		items = append(items, idNameDTO{ID: dv.ID, Name: dv.Name})
	}
	writeJSON(w, http.StatusOK, items)
}

// handleSensors implements GET /sensors?deviceId=<id>: the direct child
// sensors of a given device.
func (d *Datasource) handleSensors(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")
	if deviceID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "deviceId is required")
		return
	}

	sensors, err := prtg.FetchAll[prtg.SensorInfo](r.Context(), d.client, "/experimental/sensors", prtg.FetchAllOptions{
		Filter:   prtg.ParentID(deviceID).String(),
		MaxItems: maxPickerItems,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	items := make([]idNameDTO, 0, len(sensors.Items))
	for _, s := range sensors.Items {
		items = append(items, idNameDTO{ID: s.ID, Name: s.Name})
	}
	writeJSON(w, http.StatusOK, items)
}

// handleChannels implements GET /channels?sensorId=<id>: the channels of a
// given sensor.
func (d *Datasource) handleChannels(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensorId")
	if sensorID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "sensorId is required")
		return
	}

	channels, err := prtg.FetchAll[prtg.ChannelInfo](r.Context(), d.client, "/experimental/channels", prtg.FetchAllOptions{
		Filter:   prtg.ParentID(sensorID).String(),
		MaxItems: maxPickerItems,
	})
	if err != nil {
		writeUpstreamError(w, err)
		return
	}

	items := make([]channelDTO, 0, len(channels.Items))
	for _, ch := range channels.Items {
		items = append(items, channelDTO{ID: ch.ID, Name: ch.Name, Unit: ch.Unit.DisplayUnit})
	}
	writeJSON(w, http.StatusOK, items)
}

// handleSensorsSearch implements
// GET sensors/search?pattern=<regex>&groupId=&deviceId=&channelMatchMode=&channelPattern=
func (d *Datasource) handleSensorsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pattern := q.Get("pattern")
	if pattern == "" {
		writeErrorJSON(w, http.StatusBadRequest, "pattern is required")
		return
	}

	params := prtg.SearchSensorsParams{
		Pattern:          pattern,
		GroupID:          q.Get("groupId"),
		DeviceID:         q.Get("deviceId"),
		ChannelMatchMode: q.Get("channelMatchMode"),
		ChannelPattern:   q.Get("channelPattern"),
	}

	result, err := prtg.SearchSensors(r.Context(), d.client, params)
	if err != nil {
		if errors.Is(err, prtg.ErrInvalidPattern) {
			writeErrorJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		writeUpstreamError(w, err)
		return
	}

	sensors := make([]sensorMatchDTO, 0, len(result.Sensors))
	for _, s := range result.Sensors {
		channels := make([]channelMatchDTO, 0, len(s.MatchedChannels))
		for _, ch := range s.MatchedChannels {
			channels = append(channels, channelMatchDTO{ID: ch.ID, Name: ch.Name})
		}
		sensors = append(sensors, sensorMatchDTO{
			ID:              s.ID,
			Name:            s.Name,
			DeviceID:        s.DeviceID,
			DeviceName:      s.DeviceName,
			MatchedChannels: channels,
		})
	}

	writeJSON(w, http.StatusOK, sensorSearchResponseDTO{
		Sensors:       sensors,
		TotalSensors:  result.TotalSensors,
		TotalChannels: result.TotalChannels,
		Truncated:     result.Truncated,
	})
}
