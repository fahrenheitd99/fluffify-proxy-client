//go:build windows

package tui

import (
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi     = windows.NewLazySystemDLL("iphlpapi.dll")
	getIfTable2  = iphlpapi.NewProc("GetIfTable2")
	freeMibTable = iphlpapi.NewProc("FreeMibTable")
)

// mibIfRow2 повторяет MIB_IF_ROW2 из iphlpapi.h (x64). Структура описана
// целиком: InOctets/OutOctets лежат после цепочки ULONG64, поэтому вычислить
// их смещение «на глаз» нельзя. Размер и смещения проверяет speed_windows_test.go.
type mibIfRow2 struct {
	InterfaceLuid                  uint64
	InterfaceIndex                 uint32
	_                              uint32
	InterfaceGuid                  windows.GUID
	Alias                          [257]uint16
	Description                    [257]uint16
	PhysicalAddressLength          uint32
	PhysicalAddress                [32]byte
	PermanentPhysicalAddressLength uint32
	PermanentPhysicalAddress       [32]byte
	Mtu                            uint32
	Type                           uint32
	TunnelType                     uint32
	MediaType                      uint32
	PhysicalMediumType             uint32
	AccessType                     uint32
	Direction                      uint32
	Flags                          byte
	_                              [3]byte
	OperStatus                     uint32
	AdminStatus                    uint32
	MediaConnectState              uint32
	NetworkGuid                    windows.GUID
	ConnectionType                 uint32
	_                              uint32 // CompartmentId + выравнивание под uint64
	TransmitLinkSpeed              uint64
	ReceiveLinkSpeed               uint64
	InOctets                       uint64
	InUcastPkts                    uint64
	InNUcastPkts                   uint64
	InDiscards                     uint64
	InErrors                       uint64
	InUnknownProtos                uint64
	InUcastOctets                  uint64
	InMulticastOctets              uint64
	InBroadcastOctets              uint64
	OutOctets                      uint64
	OutUcastPkts                   uint64
	OutNUcastPkts                  uint64
	OutDiscards                    uint64
	OutErrors                      uint64
	OutUcastOctets                 uint64
	OutMulticastOctets             uint64
	OutBroadcastOctets             uint64
	OutQLen                        uint64
}

type mibIfTable2 struct {
	NumEntries uint32
	_          uint32
	Table      [1]mibIfRow2
}

type ifStats struct {
	alias string
	rx    uint64
	tx    uint64
}

// readIfTable запрашивает у iphlpapi счётчики всех интерфейсов.
func readIfTable() []ifStats {
	var table *mibIfTable2
	if status, _, _ := getIfTable2.Call(uintptr(unsafe.Pointer(&table))); status != 0 || table == nil {
		return nil
	}
	defer freeMibTable.Call(uintptr(unsafe.Pointer(table)))

	if table.NumEntries == 0 || table.NumEntries > 4096 {
		return nil
	}

	rows := unsafe.Slice(&table.Table[0], int(table.NumEntries))
	out := make([]ifStats, 0, len(rows))
	for i := range rows {
		alias := trimUTF16(rows[i].Alias[:])
		if alias == "" {
			continue
		}
		out = append(out, ifStats{alias: alias, rx: rows[i].InOctets, tx: rows[i].OutOctets})
	}
	return out
}

func readNetStats() map[string][2]uint64 {
	st := readIfTable()
	if len(st) == 0 {
		return nil
	}
	res := make(map[string][2]uint64, len(st))
	for _, s := range st {
		res[s.alias] = [2]uint64{s.rx, s.tx}
	}
	return res
}

// defaultRouteIface на Windows выбирает самый нагруженный физический интерфейс:
// таблица маршрутов через iphlpapi заметно громоздче, а для индикатора скорости
// этого достаточно.
func defaultRouteIface(exclude string) string {
	var best string
	var bestTotal uint64
	for _, s := range readIfTable() {
		if isIgnoredIface(s.alias, exclude) {
			continue
		}
		if total := s.rx + s.tx; best == "" || total > bestTotal {
			best = s.alias
			bestTotal = total
		}
	}
	return best
}

func trimUTF16(s []uint16) string {
	for i, v := range s {
		if v == 0 {
			s = s[:i]
			break
		}
	}
	return string(utf16.Decode(s))
}
