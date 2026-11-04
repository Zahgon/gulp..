package vinyl

import "reflect"

// deepCopy returns a copy of v that shares no maps, slices, arrays or pointers
// with it. It is the Go counterpart of the npm `clone` package that vinyl's
// clone() applies to custom properties when deep is set.
//
// Like `clone`, it keeps shared and circular references intact: a value
// reached twice through the same pointer, map or slice is copied once and the
// copy is reached twice. Nested vinyl files are copied with Clone so that their
// own invariants (history, base, contents) are preserved. Unexported struct
// fields are copied by value, which is as deep as reflection can reach.
func deepCopy(v any) any {
	if v == nil {
		return nil
	}
	c := copier{seen: map[visit]reflect.Value{}}
	return c.copy(reflect.ValueOf(v)).Interface()
}

// visit identifies a reference value already copied: its address and type,
// since a struct and its first field share an address.
type visit struct {
	ptr uintptr
	typ reflect.Type
}

type copier struct {
	seen map[visit]reflect.Value
}

var fileType = reflect.TypeOf((*File)(nil))

func (c *copier) copy(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		key := visit{v.Pointer(), v.Type()}
		if done, ok := c.seen[key]; ok {
			return done
		}
		if v.Type() == fileType {
			dup := reflect.ValueOf(v.Interface().(*File).Clone())
			c.seen[key] = dup
			return dup
		}
		dup := reflect.New(v.Type().Elem())
		c.seen[key] = dup
		dup.Elem().Set(c.copy(v.Elem()))
		return dup
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		dup := reflect.New(v.Type()).Elem()
		dup.Set(c.copy(v.Elem()))
		return dup
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		key := visit{v.Pointer(), v.Type()}
		if done, ok := c.seen[key]; ok {
			return done
		}
		dup := reflect.MakeMapWithSize(v.Type(), v.Len())
		c.seen[key] = dup
		iter := v.MapRange()
		for iter.Next() {
			dup.SetMapIndex(c.copy(iter.Key()), c.copy(iter.Value()))
		}
		return dup
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		key := visit{v.Pointer(), v.Type()}
		if done, ok := c.seen[key]; ok {
			return done
		}
		dup := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		c.seen[key] = dup
		for i := range v.Len() {
			dup.Index(i).Set(c.copy(v.Index(i)))
		}
		return dup
	case reflect.Array:
		dup := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			dup.Index(i).Set(c.copy(v.Index(i)))
		}
		return dup
	case reflect.Struct:
		dup := reflect.New(v.Type()).Elem()
		dup.Set(v)
		for i := range v.NumField() {
			if dup.Field(i).CanSet() {
				dup.Field(i).Set(c.copy(v.Field(i)))
			}
		}
		return dup
	default:
		return v
	}
}
