package elb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
)

// fakeELB is an in-memory ELBv3 API subset backing the lifecycle tests.
type fakeELB struct {
	mu sync.Mutex

	server *httptest.Server

	nextID int

	loadBalancers map[string]map[string]any // id -> object
	listeners     map[string]map[string]any
	pools         map[string]map[string]any
	monitors      map[string]map[string]any
	// members: poolID -> memberID -> object
	members map[string]map[string]map[string]any
	// eips: EIP id -> object (network v1 publicips API)
	eips map[string]map[string]any
	// ipGroups: group id -> object
	ipGroups map[string]map[string]any
}

func newFakeELB() *fakeELB {
	f := &fakeELB{
		loadBalancers: map[string]map[string]any{},
		listeners:     map[string]map[string]any{},
		pools:         map[string]map[string]any{},
		monitors:      map[string]map[string]any{},
		members:       map[string]map[string]map[string]any{},
		eips:          map[string]map[string]any{},
		ipGroups:      map[string]map[string]any{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeELB) close() { f.server.Close() }

func (f *fakeELB) client() *golangsdk.ServiceClient {
	return &golangsdk.ServiceClient{
		ProviderClient: &golangsdk.ProviderClient{},
		Endpoint:       f.server.URL + "/",
		ResourceBase:   f.server.URL + "/elb/",
	}
}

// networkClient mimics a NewNetworkV1 client (URLs: <base>/<project>/publicips).
func (f *fakeELB) networkClient() *golangsdk.ServiceClient {
	return &golangsdk.ServiceClient{
		ProviderClient: &golangsdk.ProviderClient{ProjectID: "test-project"},
		Endpoint:       f.server.URL + "/net/",
		ResourceBase:   f.server.URL + "/net/v1/",
	}
}

func (f *fakeELB) genID(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s-%04d", prefix, f.nextID)
}

func (f *fakeELB) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Network v1 API (EIPs): /net/v1/<project>/publicips[/<id>]
	if strings.HasPrefix(r.URL.Path, "/net/") {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/net/v1/"), "/"), "/")
		if len(parts) >= 2 && parts[1] == "publicips" {
			f.handleEips(w, r, parts[2:])
			return
		}
		http.Error(w, `{"error": "unknown network path"}`, http.StatusNotFound)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/elb/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	switch {
	case parts[0] == "loadbalancers":
		f.handleLoadBalancers(w, r, parts[1:])
	case parts[0] == "listeners":
		f.handleListeners(w, r, parts[1:])
	case parts[0] == "pools" && len(parts) >= 3 && parts[2] == "members":
		f.handleMembers(w, r, parts)
	case parts[0] == "pools":
		f.handlePools(w, r, parts[1:])
	case parts[0] == "healthmonitors":
		f.handleMonitors(w, r, parts[1:])
	case parts[0] == "ipgroups":
		f.handleIPGroups(w, r, parts[1:])
	default:
		http.Error(w, `{"error": "unknown path `+r.URL.Path+`"}`, http.StatusNotFound)
	}
}

func (f *fakeELB) handleLoadBalancers(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodPost:
		body := readBody(r, "loadbalancer")
		id := f.genID("lb")
		body["id"] = id
		body["provisioning_status"] = "ACTIVE"
		body["operating_status"] = "ONLINE"
		if body["vip_address"] == nil || body["vip_address"] == "" {
			body["vip_address"] = "192.168.0.100"
		}
		// A nested publicip block creates and binds a new EIP.
		if _, ok := body["publicip"]; ok {
			eipID := f.genID("eip")
			addr := fmt.Sprintf("80.158.0.%d", f.nextID)
			f.eips[eipID] = map[string]any{"id": eipID, "public_ip_address": addr, "port_id": ""}
			body["publicips"] = []any{map[string]any{"publicip_id": eipID, "publicip_address": addr, "ip_version": 4}}
			delete(body, "publicip")
		}
		f.loadBalancers[id] = body
		writeJSON(w, http.StatusCreated, map[string]any{"loadbalancer": body})
	case len(rest) == 0 && r.Method == http.MethodGet:
		name := r.URL.Query().Get("name")
		var items []map[string]any
		for _, lb := range f.loadBalancers {
			if name == "" || lb["name"] == name {
				items = append(items, lb)
			}
		}
		writeList(w, "loadbalancers", items)
	case len(rest) == 1 && r.Method == http.MethodGet:
		lb, ok := f.loadBalancers[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"loadbalancer": lb})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		if _, ok := f.loadBalancers[rest[0]]; !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		delete(f.loadBalancers, rest[0])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func (f *fakeELB) handleListeners(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodPost:
		body := readBody(r, "listener")
		id := f.genID("listener")
		body["id"] = id
		body["default_pool_id"] = ""
		f.listeners[id] = body
		writeJSON(w, http.StatusCreated, map[string]any{"listener": body})
	case len(rest) == 0 && r.Method == http.MethodGet:
		lbID := r.URL.Query().Get("loadbalancer_id")
		var items []map[string]any
		for _, l := range f.listeners {
			if lbID == "" || l["loadbalancer_id"] == lbID {
				items = append(items, l)
			}
		}
		writeList(w, "listeners", items)
	case len(rest) == 1 && r.Method == http.MethodGet:
		l, ok := f.listeners[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"listener": l})
	case len(rest) == 1 && r.Method == http.MethodPut:
		l, ok := f.listeners[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		for k, v := range readBody(r, "listener") {
			l[k] = v
		}
		writeJSON(w, http.StatusOK, map[string]any{"listener": l})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		if _, ok := f.listeners[rest[0]]; !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		delete(f.listeners, rest[0])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func (f *fakeELB) handlePools(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodPost:
		body := readBody(r, "pool")
		id := f.genID("pool")
		body["id"] = id
		body["healthmonitor_id"] = ""
		if listenerID, _ := body["listener_id"].(string); listenerID != "" {
			if l, ok := f.listeners[listenerID]; ok {
				l["default_pool_id"] = id
			}
		}
		f.pools[id] = body
		f.members[id] = map[string]map[string]any{}
		writeJSON(w, http.StatusCreated, map[string]any{"pool": body})
	case len(rest) == 1 && r.Method == http.MethodGet:
		p, ok := f.pools[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pool": p})
	case len(rest) == 1 && r.Method == http.MethodPut:
		p, ok := f.pools[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		for k, v := range readBody(r, "pool") {
			p[k] = v
		}
		writeJSON(w, http.StatusOK, map[string]any{"pool": p})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		if _, ok := f.pools[rest[0]]; !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		delete(f.pools, rest[0])
		delete(f.members, rest[0])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func (f *fakeELB) handleMembers(w http.ResponseWriter, r *http.Request, parts []string) {
	// parts: ["pools", poolID, "members"] or ["pools", poolID, "members", memberID]
	poolID := parts[1]
	pool, ok := f.members[poolID]
	if !ok {
		http.Error(w, `{"error": "pool not found"}`, http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 3 && r.Method == http.MethodPost:
		body := readBody(r, "member")
		id := f.genID("member")
		body["id"] = id
		body["pool_id"] = poolID
		pool[id] = body
		writeJSON(w, http.StatusCreated, map[string]any{"member": body})
	case len(parts) == 3 && r.Method == http.MethodGet:
		var items []map[string]any
		for _, m := range pool {
			items = append(items, m)
		}
		writeList(w, "members", items)
	case len(parts) == 4 && r.Method == http.MethodDelete:
		if _, ok := pool[parts[3]]; !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		delete(pool, parts[3])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func (f *fakeELB) handleMonitors(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodPost:
		body := readBody(r, "healthmonitor")
		id := f.genID("monitor")
		body["id"] = id
		if poolID, _ := body["pool_id"].(string); poolID != "" {
			if p, ok := f.pools[poolID]; ok {
				p["healthmonitor_id"] = id
			}
		}
		f.monitors[id] = body
		writeJSON(w, http.StatusCreated, map[string]any{"healthmonitor": body})
	case len(rest) == 1 && r.Method == http.MethodGet:
		m, ok := f.monitors[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"healthmonitor": m})
	case len(rest) == 1 && r.Method == http.MethodPut:
		m, ok := f.monitors[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		for k, v := range readBody(r, "healthmonitor") {
			m[k] = v
		}
		writeJSON(w, http.StatusOK, map[string]any{"healthmonitor": m})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		m, ok := f.monitors[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		if poolID, _ := m["pool_id"].(string); poolID != "" {
			if p, ok := f.pools[poolID]; ok {
				p["healthmonitor_id"] = ""
			}
		}
		delete(f.monitors, rest[0])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func (f *fakeELB) handleEips(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 1 && r.Method == http.MethodGet:
		eip, ok := f.eips[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"publicip": eip})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		if _, ok := f.eips[rest[0]]; !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		delete(f.eips, rest[0])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func (f *fakeELB) handleIPGroups(w http.ResponseWriter, r *http.Request, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodPost:
		body := readBody(r, "ipgroup")
		id := f.genID("ipgroup")
		body["id"] = id
		f.ipGroups[id] = body
		writeJSON(w, http.StatusCreated, map[string]any{"ipgroup": body})
	case len(rest) == 1 && r.Method == http.MethodGet:
		g, ok := f.ipGroups[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ipgroup": g})
	case len(rest) == 1 && r.Method == http.MethodPut:
		g, ok := f.ipGroups[rest[0]]
		if !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		for k, v := range readBody(r, "ipgroup") {
			g[k] = v
		}
		writeJSON(w, http.StatusOK, map[string]any{"ipgroup": g})
	case len(rest) == 1 && r.Method == http.MethodDelete:
		if _, ok := f.ipGroups[rest[0]]; !ok {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
			return
		}
		delete(f.ipGroups, rest[0])
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error": "unsupported"}`, http.StatusMethodNotAllowed)
	}
}

func readBody(r *http.Request, envelope string) map[string]any {
	var wrapper map[string]map[string]any
	_ = json.NewDecoder(r.Body).Decode(&wrapper)
	body := wrapper[envelope]
	if body == nil {
		body = map[string]any{}
	}
	return body
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeList(w http.ResponseWriter, key string, items []map[string]any) {
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		key:         items,
		"page_info": map[string]any{"next_marker": "", "current_count": len(items)},
	})
}
