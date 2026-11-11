package handler

import (
	"encoding/json"
	"net/http"

	"github.com/seongJae/owlmon/server/db"
)

var topologyKinds = map[string]bool{"internet": true, "firewall": true, "network": true, "server": true}

type TopologyHandler struct {
	store *db.TopologyStore
}

func NewTopologyHandler(store *db.TopologyStore) *TopologyHandler {
	return &TopologyHandler{store: store}
}

// GET /api/topology
func (h *TopologyHandler) Get(w http.ResponseWriter, r *http.Request) {
	l, err := h.store.Get(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(l)
}

// PUT /api/topology
func (h *TopologyHandler) Save(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var l db.TopologyLayout
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		http.Error(w, "잘못된 요청 형식", http.StatusBadRequest)
		return
	}
	if msg := validateLayout(l); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if l.Zones == nil {
		l.Zones = []string{}
	}
	if l.Devices == nil {
		l.Devices = []db.TopologyDevice{}
	}
	if err := h.store.Save(r.Context(), l); err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateLayout(l db.TopologyLayout) string {
	zones := map[string]bool{}
	for _, z := range l.Zones {
		if z == "" || zones[z] {
			return "구역 이름이 비었거나 중복되었습니다"
		}
		zones[z] = true
	}
	parent := map[string]string{}
	paired := map[string]int{}
	for _, d := range l.Devices {
		if d.Name == "" {
			return "장비 이름이 비었습니다"
		}
		if _, dup := parent[d.Name]; dup {
			return "장비 이름이 중복되었습니다: " + d.Name
		}
		if !topologyKinds[d.Kind] {
			return "알 수 없는 장비 종류입니다: " + d.Kind
		}
		if !zones[d.Zone] {
			return "등록되지 않은 구역입니다: " + d.Zone
		}
		parent[d.Name] = d.Parent
		if d.Pair != "" {
			paired[d.Name]++
			paired[d.Pair]++
		}
	}
	for _, d := range l.Devices {
		if _, ok := parent[d.Parent]; d.Parent != "" && !ok {
			return "상위 장비를 찾을 수 없습니다: " + d.Parent
		}
		if _, ok := parent[d.Pair]; d.Pair != "" && (!ok || d.Pair == d.Name) {
			return "이중화 짝 장비가 잘못되었습니다: " + d.Pair
		}
		// 양쪽에 서로 적은 경우(2)까지 허용, 그보다 많으면 한 장비가 여러 짝에 묶인 것
		if paired[d.Name] > 2 || (paired[d.Name] == 2 && !mutualPair(l.Devices, d)) {
			return "한 장비는 이중화 짝을 하나만 가질 수 있습니다: " + d.Name
		}
		// 상위 장비를 따라 올라가다 자기 자신을 다시 만나면 순환
		seen := map[string]bool{}
		for n := d.Name; n != ""; n = parent[n] {
			if seen[n] {
				return "상위 장비 연결이 순환합니다: " + d.Name
			}
			seen[n] = true
		}
	}
	return ""
}

func mutualPair(devices []db.TopologyDevice, d db.TopologyDevice) bool {
	for _, m := range devices {
		if m.Name == d.Pair {
			return m.Pair == d.Name
		}
	}
	return false
}
