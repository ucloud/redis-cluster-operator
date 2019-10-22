package redisutil

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var slotSeparator = "-"
var importingSeparator = "-<-"
var migratingSeparator = "->-"

// Slot represent a Redis Cluster slot
type Slot uint64

// String string representation of a slot
func (s Slot) String() string {
	return strconv.FormatUint(uint64(s), 10)
}

// SlotSlice attaches the methods of sort.Interface to []string, sorting in increasing order.
type SlotSlice []Slot

func (s SlotSlice) Len() int           { return len(s) }
func (s SlotSlice) Less(i, j int) bool { return s[i] < s[j] }
func (s SlotSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s SlotSlice) String() string {
	return fmt.Sprintf("%s", SlotRangesFromSlots(s))
}

// DecodeSlot parse a string representation of a slot slot
func DecodeSlot(s string) (Slot, error) {
	slot, err := strconv.ParseUint(s, 10, 64)
	return Slot(slot), err
}

// SlotRange represent a Range of slots
type SlotRange struct {
	Min Slot `json:"min"`
	Max Slot `json:"max"`
}

// String string representation of a slotrange
func (s SlotRange) String() string {
	return s.Min.String() + slotSeparator + s.Max.String()
}

// Total returns total slot present in the range
func (s SlotRange) Total() int {
	return int(s.Max - s.Min + 1)
}

// ImportingSlot represents an importing slot (slot + importing from node id)
type ImportingSlot struct {
	SlotID     Slot   `json:"slot"`
	FromNodeID string `json:"fromNodeId"`
}

// String string representation of an importing slot
func (s ImportingSlot) String() string {
	return s.SlotID.String() + importingSeparator + s.FromNodeID
}

// MigratingSlot represents a migrating slot (slot + migrating to node id)
type MigratingSlot struct {
	SlotID   Slot   `json:"slot"`
	ToNodeID string `json:"toNodeId"`
}

// String string representation of a migratting slot
func (s MigratingSlot) String() string {
	return s.SlotID.String() + migratingSeparator + s.ToNodeID
}

// DecodeSlotRange decode from a string a RangeSlot
//  each entry can have 4 representations:
//       * single slot: ex: 42
//       * slot range: ex: 42-52
//       * migrating slot: ex: [42->-67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1]
//       * importing slot: ex: [42-<-67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1]
func DecodeSlotRange(str string) ([]Slot, *ImportingSlot, *MigratingSlot, error) {
	val := strings.Split(str, slotSeparator)
	var min, max, slot Slot
	slots := []Slot{}
	var err error
	if len(val) == 3 {
		// migrating or importing slot
		separator := slotSeparator + val[1] + slotSeparator
		slot, err = DecodeSlot(strings.TrimPrefix(val[0], "["))
		if err != nil {
			return slots, nil, nil, err
		}
		if separator == importingSeparator {
			return slots, &ImportingSlot{SlotID: slot, FromNodeID: strings.TrimSuffix(val[2], "]")}, nil, err
		} else if separator == migratingSeparator {
			return slots, nil, &MigratingSlot{SlotID: slot, ToNodeID: strings.TrimSuffix(val[2], "]")}, err
		} else {
			return slots, nil, nil, fmt.Errorf("impossible to decode slot %s", str)
		}
	} else if len(val) > 0 {
		min, err = DecodeSlot(val[0])
		if err != nil {
			return slots, nil, nil, err
		}
		if len(val) > 1 {
			max, err = DecodeSlot(val[1])
			if err != nil {
				return slots, nil, nil, err
			}
		} else {
			max = min
		}
	} else {
		return slots, nil, nil, fmt.Errorf("impossible to decode slot '%s'", str)
	}

	slots = BuildSlotSlice(min, max)

	return slots, nil, nil, err
}

// SlotRangesFromSlots return a slice of slot ranges from a slice of slots
func SlotRangesFromSlots(slots []Slot) []SlotRange {
	ranges := []SlotRange{}
	min := Slot(0)
	max := Slot(0)
	first := true
	sort.Sort(SlotSlice(slots))
	for _, slot := range slots {
		if first {
			min = slot
			max = slot
			first = false
			continue
		}
		if slot > max+1 {
			ranges = append(ranges, SlotRange{Min: min, Max: max})
			min = slot
		}
		max = slot
	}
	if !first {
		ranges = append(ranges, SlotRange{Min: min, Max: max})
	}

	return ranges
}

// RemoveSlots return a new list of slot where a list of slots have been removed, doesn't work if duplicates
func RemoveSlots(slots []Slot, removedSlots []Slot) []Slot {
	for i := 0; i < len(slots); i++ {
		s := slots[i]
		for _, r := range removedSlots {
			if s == r {
				slots = append(slots[:i], slots[i+1:]...)
				i--
				break
			}
		}
	}

	return slots
}

// RemoveSlot return a new list of slot where a specified slot have been removed.
func RemoveSlot(slots []Slot, removedSlot Slot) []Slot {
	for i := 0; i < len(slots); i++ {
		s := slots[i]
		if s == removedSlot {
			slots = append(slots[:i], slots[i+1:]...)
			break
		}
	}

	return slots
}

// AddSlots return a new list of slots after adding some slots in it, duplicates are removed
func AddSlots(slots []Slot, addedSlots []Slot) []Slot {
	for _, s := range addedSlots {
		if !Contains(slots, s) {
			slots = append(slots, s)
		}
	}
	return slots
}

// Contains returns true if a node slice contains a node
func Contains(s []Slot, e Slot) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// BuildSlotSlice return a slice of all slots between this range
func BuildSlotSlice(min, max Slot) []Slot {
	slots := []Slot{}
	for s := min; s <= max; s++ {
		slots = append(slots, s)
	}
	return slots
}
