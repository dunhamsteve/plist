package plist

import (
	"unicode/utf16"
	"strings"
	"fmt"
	"bytes"
	"reflect"
	"encoding/binary"
)

// Quick and dirty binary plist encoder, doesn't reuse values, and sometimes uses wider encodings than necessary
func Marshal(data interface{}) ([]byte, error) {
	
	var e encoder
	e.buf = bytes.NewBuffer([]byte("bplist00"))
	e.objects = append(e.objects, data)
	e.strings = make( map[string]uint16)
	for i:= 0; i < len(e.objects); i++ {
		e.offsets = append(e.offsets, e.buf.Len())
		e.writeObject(e.objects[i])
	}
	offsetStart := uint64(e.buf.Len())
	for _,v := range e.offsets {
		off := uint32(v)
		binary.Write(e.buf, binary.BigEndian, &off)
	}
	
	
	var blank [6]byte
	e.buf.Write(blank[:])
	
	e.buf.WriteByte(4)
	e.buf.WriteByte(2)
	x := uint64(len(e.objects))
	binary.Write(e.buf, binary.BigEndian, x)
	x = 0
	binary.Write(e.buf, binary.BigEndian, x)
	binary.Write(e.buf, binary.BigEndian, offsetStart)
	
	return e.buf.Bytes(), nil
}

func (e *encoder) tag(a byte, b int) {
	if b < 15 {
		e.buf.WriteByte( a << 4 | byte(b & 0xf))
	} else {
		e.buf.WriteByte( a << 4 | 0xf)
		e.writeObject(b)
	}
}

func (e *encoder) writeObject(o interface{}) {
	e.writeValue(reflect.ValueOf(o))
}

func (e *encoder) writeValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Ptr:
		e.writeValue(v.Elem())
		return
	case reflect.Map:
		keys := v.MapKeys()
		e.tag(13,len(keys))
		for _,k := range keys {
			e.writeRef(k.Interface())
		}
		for _,k := range keys {
			e.writeRef(v.MapIndex(k).Interface())
		}
		return
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value := uint64(v.Int())
		if value < 256 {
			e.tag(1,0)
			e.buf.WriteByte(byte(value))
		} else {
			e.tag(1,3)
			binary.Write(e.buf, binary.BigEndian, value)
		}
		return
	case reflect.Bool:
		if v.Bool() {
			e.tag(0,9)
		} else {
			e.tag(0,8)
		}
		return
	case reflect.Slice:
		l := v.Len()
		e.tag(10, l)
		for i := 0; i < l; i++ {
			e.writeRef(v.Index(i).Interface())
		}
		return
	case reflect.String:
		s := []byte(v.String())
		ascii := true
		for _,c := range s {
			ascii = ascii && c < 128
		}
		// Sheesh, maybe we should just stick with the XML encoding..
		if ascii {
			e.tag(5,len(s))
			e.buf.Write(s)
		} else {
			s2 := utf16.Encode([]rune(v.String()))
			e.tag(6,len(s2))
			binary.Write(e.buf, binary.BigEndian, s2)
		}
		return
	case reflect.Float64, reflect.Float32:
		value := v.Float()
		e.tag(2, 3)
		binary.Write(e.buf, binary.BigEndian, value)
		return
	case reflect.Struct:
		// find field
		typ := v.Type()
		e.tag(13,v.NumField())
		for j := 0; j < v.NumField(); j++ {
			f := typ.Field(j)
			tag := f.Tag.Get("plist")
			if len(tag) == 0 {
				tag = strings.ToLower(f.Name[:1])+f.Name[1:]
			}
			e.writeRef(tag)
		}
		for j := 0; j < v.NumField(); j++ {
			f := typ.Field(j)
			value := v.FieldByIndex(f.Index) 
			e.writeRef(value.Interface())
		}
		return
	}
	
	fmt.Println("Unhandled kind", v.Kind())
}

// queue an object for writing and insert a reference.  We reuse strings, but that's it.
func (e *encoder) writeRef(o interface{}) {
	if s, isstring := o.(string); isstring {
		if id, ok := e.strings[s]; ok {
			binary.Write(e.buf, binary.BigEndian, id)
			return
		}
	}
	id := uint16(len(e.objects))
	e.objects = append(e.objects, o)
	binary.Write(e.buf, binary.BigEndian, id)
	if s, isstring := o.(string); isstring {
		e.strings[s] = id
	}
}


type encoder struct {
	buf *bytes.Buffer
	objects []interface{}
	offsets []int
	strings map[string]uint16
}

