package plist

// very simple plist reader/writer
// for now, we just reflect into generic data structures
import (
	"unicode/utf16"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type decoder struct {
	r            io.ReadSeeker
	offsetSize   byte
	refSize      byte
	objCount     uint64
	topObject    int64
	offsetOffset int64
	err          error
	offsets      []int64
	objects      []interface{}
}

func indirect(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Ptr {
		return v
	}
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return v.Elem()
}

func (s *decoder) read(size byte) (v int64) {
	for {
		size--
		t := s.readByte()
		v |= (int64(t) & 0xff) << (size * 8)
		if size == 0 {
			break
		}
	}
	return v
}

func (s *decoder) readByte() byte {
	var t byte
	binary.Read(s.r, binary.BigEndian, &t)
	return t
}

func (s *decoder) readRef(t reflect.Value) {
	idx := s.read(s.refSize)
	s.get(idx, t)
}

func (s *decoder) get(idx int64, v reflect.Value) {
	pos, _ := s.r.Seek(0, 1)
	s.r.Seek(s.offsets[idx], 0)
	s.readObject(v)
	s.r.Seek(pos, 0)
	return
}

func (s *decoder) readDict(vv reflect.Value, count int) {
	// this is where the magic happens..
	var ismap bool
	v := indirect(vv)
	
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			// Can this be another type? (e.g. number, date, etc)
			v.Set(reflect.ValueOf(make(map[string]interface{})))
		}
		v = v.Elem()
	}
	
	switch v.Kind() {
	default:
		fmt.Println(v.Kind(), v, vv.Kind(), vv, v.IsValid())
		panic("Can't decode Map")
	case reflect.Map:
		t := v.Type()
		if t.Key() != reflect.TypeOf("") {
			panic("map must have string key")
		}
		ismap = true
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	case reflect.Struct:
		ismap = false
	}

	keys := make([]string, count)
	keysV := reflect.ValueOf(keys)
	for i := 0; i < count; i++ {
		s.readRef(keysV.Index(i))
	}

	var dest reflect.Value
	for i := 0; i < count; i++ {
		if ismap {
			eType := v.Type().Elem()
			dest = reflect.New(eType).Elem()
		} else {
			var field reflect.StructField
			// find field
			var ok bool
			typ := v.Type()
			for j := 0; j < v.NumField(); j++ {
				f := typ.Field(j)
				tag := f.Tag.Get("plist")
				if field.Anonymous {
					continue // recurse?
				}
				if tag == keys[i] {
					field, ok = f, true
					break
				}
				if strings.EqualFold(f.Name, keys[i]) {
					field, ok = f, true
				}
			}
			if ok {
				dest = v.FieldByIndex(field.Index)
			} else {
				fmt.Println("nowhere to stick", keys[i])
				panic(nil)
			}
		}
		s.readRef(dest)
		if ismap {
			v.SetMapIndex(keysV.Index(i), dest)
		}
	}
}

func (s *decoder) readArray(v reflect.Value, count int) {
	var vv reflect.Value
	switch v.Kind() {
	default:
		fmt.Println(v)
		panic("Can't decode array")
	case reflect.Interface:
		v.Set(reflect.ValueOf(make([]interface{},0)))
		vv = v.Elem()
	//case reflect.Array:
	case reflect.Slice:
		vv = v
	}
	vv = reflect.MakeSlice(vv.Type(), count, count)
	v.Set(vv)

	for i := 0; i < count; i++ {
		s.readRef(vv.Index(i))
	}
	return
}

type UID []byte

func (uid UID) Value() (rval uint) {
	for _,x := range uid {
		rval <<= 8
		rval |= uint(x)
	}
	return
}

func (uid UID) String() string {
	return fmt.Sprintf("UID:%x", []byte(uid))
}

func (s *decoder) readObject(v reflect.Value) {
	tag := s.readByte()
	a := tag >> 4
	b := uint64(tag & 0xf)
	if b == 15 {
		s.readObject(reflect.ValueOf(&b))
	}
	switch a {
	case 0:
		switch b {
		case 0: // nil - FIXME - not sure if it acutally sets nil in the right cases
			v.Set(reflect.Zero(v.Type()))
			return
		case 8:
			v.Set(reflect.ValueOf(false))
			return
		case 9:
			v.Set(reflect.ValueOf(true))
			return
		}
	case 1: // integer
		v = indirect(v)
		value := s.read(1 << b)
		switch v.Kind() {
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			v.SetUint(uint64(value))
		default:
			v.SetInt(value)
		}
		return
	case 2: // float
		v = indirect(v)
		var value float64
		switch b {
		case 2:
			var vv float32
			binary.Read(s.r, binary.BigEndian, &vv)
			value = float64(vv)
		case 3:
			binary.Read(s.r, binary.BigEndian, &value)
		}
		// SetFloat is probably faster, and we can use it in the non-reflect.Interface case
		v.Set(reflect.ValueOf(value))
		return
	case 4: // data
		var tmp = make([]byte, b+1)
		io.ReadFull(s.r, tmp)
		v.Set(reflect.ValueOf(tmp))
		return
	case 5: // string
		var tmp = make([]byte, b)
		io.ReadFull(s.r, tmp)
		// maybe need to be smarter here?
		v.Set(reflect.ValueOf(string(tmp)))
		return
	case 6: // unicode string
		var tmp = make([]uint16,b)
		binary.Read(s.r, binary.BigEndian, tmp)
		tmp2 := string(utf16.Decode(tmp))
		v.Set(reflect.ValueOf(tmp2))
		return
	case 8: // UID
		var tmp = UID(make([]byte, b+1))
		io.ReadFull(s.r, tmp)
		v.Set(reflect.ValueOf(tmp))
		return
	case 10: // array
		s.readArray(v, int(b))
		return
	case 13: // dict
		s.readDict(v, int(b))
		return
	}
	fmt.Printf("unhandled a=%d, b=%d\n", a, b)
	panic(nil)
	return
}

func Unmarshal(r io.ReadSeeker, target interface{}) error {
	_, err := r.Seek(0, 0)
	if err != nil {
		return err
	}
	header := make([]byte, 8)
	_, err = io.ReadFull(r, header)
	if err != nil {
		return err
	}
	if string(header) != "bplist00" {
		return errors.New("Invalid magic")
	}
	_, err = r.Seek(-26, 2)
	if err != nil {
		return err
	}

	var s decoder

	s.r = r

	binary.Read(r, binary.BigEndian, &s.offsetSize)
	binary.Read(r, binary.BigEndian, &s.refSize)
	binary.Read(r, binary.BigEndian, &s.objCount)
	binary.Read(r, binary.BigEndian, &s.topObject)
	binary.Read(r, binary.BigEndian, &s.offsetOffset)

	s.offsets = make([]int64, s.objCount)
	_, err = r.Seek(s.offsetOffset, 0)
	if err != nil {
		return err
	}

	for i := uint64(0); i < s.objCount; i++ {
		s.offsets[i] = int64(s.read(s.offsetSize))
	}

	s.get(s.topObject, reflect.ValueOf(target))
	return nil
}
