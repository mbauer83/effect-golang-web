package protobuf

// Reading a list and a map off the wire.
//
// Both are gathered rather than read in place. A repeated field may appear
// under its number more than once and in any order -- that is how an unpacked
// one is written, and a packed one may be split -- and a map is a repeated
// message, so the same is true of it. A reader that expected either together
// would be reading a message no writer promised.

import (
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// readRepeated gathers a list, which may have arrived packed, unpacked, or in
// several pieces.
func readRepeated(
	sequence structure.Sequence,
	found []occurrence,
) (dynamic.Value, bool, error) {
	list := dynamic.List{Elements: []dynamic.Value{}}
	for _, appearance := range found {
		elements, err := readElements(sequence.Element, appearance)
		if err != nil {
			return nil, false, err
		}
		list.Elements = append(list.Elements, elements...)
	}
	// An empty repeated field is written as nothing at all, so a list that was
	// not on the wire is the empty list rather than an absent field. That is
	// proto3's only way to say a list is empty, and a description asking for at
	// least one item still refuses it in the schema layer.
	return list, true, nil
}

func readElements(element structure.Node, appearance occurrence) ([]dynamic.Value, error) {
	if !packable(element) || appearance.kind != counted {
		value, err := readSingle(element, appearance)
		if err != nil {
			return nil, err
		}
		return []dynamic.Value{value}, nil
	}
	return unpacked(element.(structure.Scalar), appearance.bytes)
}

// readEntries gathers a map from the repeated message proto3 says it is.
func readEntries(
	mapping structure.Mapping,
	found []occurrence,
) (dynamic.Value, bool, error) {
	held := dynamic.Object{Fields: []dynamic.Field{}}
	for _, appearance := range found {
		if appearance.kind != counted {
			return nil, false, fmt.Errorf("a map entry is a message, and this is wire type %d",
				appearance.kind)
		}
		name, value, err := readEntry(mapping, appearance.bytes)
		if err != nil {
			return nil, false, err
		}
		held.Fields = append(held.Fields, dynamic.Field{Name: name, Value: value})
	}
	return held, true, nil
}

// readEntry reads one entry: the key in field 1 and the value in field 2.
//
// Either may be absent, which proto3 permits and means the default for its
// kind: an entry with no key is the empty string, and one with no value is the
// value type's own zero. That is the one place this codec does apply a proto3
// default, because a map entry has no other reading -- the entry is there, so
// the key and the value are there.
func readEntry(mapping structure.Mapping, bytes []byte) (string, dynamic.Value, error) {
	known := map[int]bool{1: true, 2: true}
	found, err := gathered(known, bytes)
	if err != nil {
		return "", nil, err
	}

	name := ""
	if keys := found[1]; len(keys) > 0 {
		if keys[len(keys)-1].kind != counted {
			return "", nil, fmt.Errorf("a map key is a string here, and this is wire type %d",
				keys[len(keys)-1].kind)
		}
		name = string(keys[len(keys)-1].bytes)
	}
	value, present, err := readMember(structure.Field{Node: mapping.Value, Number: 2}, found[2])
	if err != nil {
		return "", nil, err
	}
	if !present {
		// A map entry's value is always there: the entry exists, so the wire
		// omitting the value means the zero. A message-valued map with no
		// value written is the one case with nothing to put here.
		value, _ = zeroOf(mapping.Value)
	}
	return name, value, nil
}
