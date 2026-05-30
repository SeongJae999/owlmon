// topology.go — 물리 디스크 → 파티션 → LVM → 마운트 트리를 lsblk로 수집한다.
// Linux 전용. lsblk가 없거나 실패하면 nil을 반환해 graceful degrade한다.
package specs

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// BlockDevice는 블록 디바이스 트리의 한 노드다 (재귀 구조).
//
//	disk(nvme0n1) → part(nvme0n1p3) → lvm(pve-root) → mountpoint(/)
type BlockDevice struct {
	Name       string        `json:"name"`                 // nvme0n1, nvme0n1p3, pve-root
	Type       string        `json:"type"`                 // disk / part / lvm / crypt / rom
	SizeBytes  uint64        `json:"size_bytes"`           // 바이트
	FSType     string        `json:"fstype,omitempty"`     // ext4, swap, LVM2_member, ...
	Mountpoint string        `json:"mountpoint,omitempty"` // /, /boot/efi (미마운트는 빈 값)
	Rotational bool          `json:"rotational"`           // true=HDD, false=SSD (자식은 디스크값 상속)
	Model      string        `json:"model,omitempty"`      // 디스크 노드에만 채워짐
	Children   []BlockDevice `json:"children,omitempty"`
}

// lsblkNode는 lsblk -J 원시 출력 파싱용이다.
// SIZE/ROTA는 util-linux 버전에 따라 number/string/bool로 제각각이라 유연 타입으로 받는다.
type lsblkNode struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Size       flexUint64  `json:"size"`
	FSType     string      `json:"fstype"`
	Mountpoint string      `json:"mountpoint"`
	Rota       flexBool    `json:"rota"`
	Model      string      `json:"model"`
	Children   []lsblkNode `json:"children"`
}

// collectDiskTopology는 lsblk로 블록 디바이스 트리를 수집한다 (Linux 전용).
func collectDiskTopology() []BlockDevice {
	if runtime.GOOS != "linux" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// -J: JSON, -b: 바이트 단위 크기, -o: 필요한 컬럼만
	out, err := exec.CommandContext(ctx, "lsblk", "-J", "-b",
		"-o", "NAME,TYPE,SIZE,FSTYPE,MOUNTPOINT,ROTA,MODEL").Output()
	if err != nil {
		return nil // lsblk 미설치/실패 → graceful skip
	}

	var wrap struct {
		BlockDevices []lsblkNode `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &wrap); err != nil {
		return nil
	}

	var roots []BlockDevice
	for _, n := range wrap.BlockDevices {
		// 가상 디바이스 제외 (loop=스냅, ram=램디스크)
		if strings.HasPrefix(n.Name, "loop") || strings.HasPrefix(n.Name, "ram") {
			continue
		}
		roots = append(roots, convertNode(n))
	}
	return roots
}

// convertNode는 lsblk 원시 노드를 정제된 BlockDevice 트리로 변환한다.
func convertNode(n lsblkNode) BlockDevice {
	bd := BlockDevice{
		Name:       n.Name,
		Type:       n.Type,
		SizeBytes:  uint64(n.Size),
		FSType:     strings.TrimSpace(n.FSType),
		Mountpoint: strings.TrimSpace(n.Mountpoint),
		Rotational: bool(n.Rota),
		Model:      strings.TrimSpace(n.Model),
	}
	for _, c := range n.Children {
		bd.Children = append(bd.Children, convertNode(c))
	}
	return bd
}

// flexUint64는 JSON에서 number든 "string"이든 uint64로 받는다.
type flexUint64 uint64

func (f *flexUint64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		*f = flexUint64(v)
	}
	return nil // 파싱 실패해도 0으로 두고 진행 (관대)
}

// flexBool은 JSON에서 true/false든 "0"/"1"이든 bool로 받는다.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	*f = flexBool(s == "1" || s == "true")
	return nil
}
