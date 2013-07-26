package toml

import (
	"fmt"
	"reflect"
	"time"
)

// Same as PrimitiveDecode but adds a strict verification
func PrimitiveDecodeStrict(primValue Primitive,
	v interface{},
	ignore_fields map[string]interface{}) (err error) {

	err = verify(primValue, rvalue(v), ignore_fields)
	if err != nil {
		return
	}

	return
}

// The same as Decode, except that parsed data that cannot be mapped
// will throw an error
func DecodeStrict(data string,
	v interface{},
	ignore_fields map[string]interface{}) (m MetaData, err error) {

	m, err = Decode(data, v)
	if err != nil {
		return
	}

	fmt.Printf("\n-------------------\nmetadata from DecodeStrict: %r\n", m)
	err = verify(m.mapping, rvalue(v), ignore_fields)
	return
}

func CheckType(data interface{}, thestruct interface{}) (err error) {

	dType := reflect.TypeOf(data)
	dKind := dType.Kind()

	fmt.Printf("=============Checking : %s\n", dKind)
	if dKind >= reflect.Int && dKind <= reflect.Uint64 {
		return fmt.Errorf("Not implemented")
	}
	switch dKind {
	case reflect.Map:
		dataMap := data.(map[string]interface{})
		for k, v := range dataMap {
			fmt.Printf("CheckTypeMap: key=[%s] data=%r\n", k, dataMap)
			fmt.Printf("CheckTypeMap: checking subkey=[%s]\n", v)

			s := reflect.ValueOf(thestruct)
			typeOfT := s.Type()
			for i := 0; i < s.NumField(); i++ {
				f := s.Field(i)
				fmt.Printf("%d: %s %s = %v\n", i,
					typeOfT.Field(i).Name,
					f.Type(),
					f.Interface())
			}

		}
		return nil
	case reflect.Slice:
		return fmt.Errorf("Slice Not implemented")
	case reflect.String:
		return fmt.Errorf("String Not implemented")
	case reflect.Bool:
		return fmt.Errorf("Not implemented")
	case reflect.Interface:
		// we only support empty interfaces.
		if dType.NumMethod() > 0 {
			e("Unsupported type '%s'.", dKind)
		}
		return nil
	case reflect.Float32, reflect.Float64:
		return fmt.Errorf("Not implemented")
	case reflect.Array:
		return fmt.Errorf("Not implemented")
	case reflect.Struct:
		return fmt.Errorf("Not implemented")
	default:
		return fmt.Errorf("Not implemented")
	}
	return nil
}

/////////////
// verify performs a sort of type unification based on the structure of `rv`,
// which is the client representation.
//
// Any type mismatch produces an error. Finding a type that we don't know
// how to handle produces an unsupported type error.
//
// This code is patterned after the unify() function in toml/decode.go
func verify(data interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {
	// Special case. Look for a `Primitive` value.
	if rv.Type() == reflect.TypeOf((*Primitive)(nil)).Elem() {
		return verifyAnything(data, rv, ignore_fields)
	}

	// Special case. Go's `time.Time` is a struct, which we don't want
	// to confuse with a user struct.
	if rv.Type().AssignableTo(rvalue(time.Time{}).Type()) {
		return verifyDatetime(data, rv, ignore_fields)
	}

	k := rv.Kind()

	// laziness
	if k >= reflect.Int && k <= reflect.Uint64 {
		return verifyInt(data, rv, ignore_fields)
	}
	switch k {
	case reflect.Struct:
		return verifyStruct(data, rv, ignore_fields)
	case reflect.Map:
		return verifyMap(data, rv, ignore_fields)
	case reflect.Slice:
		result := verifySlice(data, rv, ignore_fields)
		return result
	case reflect.String:
		return verifyString(data, rv, ignore_fields)
	case reflect.Bool:
		return verifyBool(data, rv, ignore_fields)
	case reflect.Interface:
		// we only support empty interfaces.
		if rv.NumMethod() > 0 {
			e("Unsupported type '%s'.", rv.Kind())
		}
		result := verifyAnything(data, rv, ignore_fields)
		return result
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		return verifyFloat64(data, rv, ignore_fields)
	}
	return e("Unsupported type '%s'.", rv.Kind())
}

func verifyStruct(mapping interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	tmap, ok := mapping.(map[string]interface{})
	if !ok {
		return mismatch(rv, "map", mapping)
	}

	rt := rv.Type()

	struct_keys := make(map[string]interface{})
	for i := 0; i < rt.NumField(); i++ {

		sft := rt.Field(i)
		kname := sft.Tag.Get("toml")
		if len(kname) == 0 {
			kname = sft.Name
		}

		struct_keys[kname] = nil
	}

	for k, _ := range tmap {
		if _, ok := insensitiveGet(struct_keys, k); !ok {
			if _, ok = insensitiveGet(ignore_fields, k); !ok {
				return e("Configuration contains key [%s] "+
					"which doesn't exist in struct", k)
			}
		}
	}

	for i := 0; i < rt.NumField(); i++ {
		// A little tricky. We want to use the special `toml` name in the
		// struct tag if it exists. In particular, we need to make sure that
		// this struct field is in the current map before trying to
		// verify it.
		sft := rt.Field(i)
		kname := sft.Tag.Get("toml")
		if len(kname) == 0 {
			kname = sft.Name
		}
		if datum, ok := insensitiveGet(tmap, kname); ok {
			sf := indirect(rv.Field(i))

			// Don't try to mess with unexported types and other such things.
			if sf.CanSet() {
				if err := verify(datum, sf, ignore_fields); err != nil {
					return err
				}
			} else if len(sft.Tag.Get("toml")) > 0 {
				// Bad user! No soup for you!
				return e("Field '%s.%s' is unexported, and therefore cannot "+
					"be loaded with reflection.", rt.String(), sft.Name)
			}
		}
	}
	return nil
}

func verifyMap(mapping interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	tmap, ok := mapping.(map[string]interface{})
	if !ok {
		return badtype("map", mapping)
	}

	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}
	// Just verify each of the keys
	for _, v := range tmap {
		rvval := indirect(reflect.New(rv.Type().Elem()))
		if err := verify(v, rvval, ignore_fields); err != nil {
			return err
		}
	}
	return nil
}

func verifySlice(data interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	slice, ok := data.([]interface{})
	if !ok {
		return badtype("slice", data)
	}

	if rv.IsNil() {
		rv.Set(reflect.MakeSlice(rv.Type(), len(slice), len(slice)))
	} else {
	}

	for i, _ := range slice {
		if err := verify(slice[i], rvalue(slice[i]), ignore_fields); err != nil {
			return err
		}
	}
	return nil
}

func verifyDatetime(data interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	if _, ok := data.(time.Time); ok {
		return nil
	}
	return badtype("time.Time", data)
}

func verifyString(data interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	if _, ok := data.(string); ok {
		return nil
	}
	return badtype("string", data)
}

func verifyFloat64(data interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	if _, ok := data.(float64); ok {
		switch rv.Kind() {
		case reflect.Float32:
			fallthrough
		case reflect.Float64:
			return nil
		default:
			panic("bug")
		}
		return nil
	}
	return badtype("float", data)
}

func verifyInt(data interface{}, rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	if _, ok := data.(int64); ok {
		switch rv.Kind() {
		case reflect.Int:
			fallthrough
		case reflect.Int8:
			fallthrough
		case reflect.Int16:
			fallthrough
		case reflect.Int32:
			fallthrough
		case reflect.Int64:
			return nil

		case reflect.Uint:
			fallthrough
		case reflect.Uint8:
			fallthrough
		case reflect.Uint16:
			fallthrough
		case reflect.Uint32:
			fallthrough
		case reflect.Uint64:
			return nil

		default:
			panic("bug")
		}
		return nil
	}
	return badtype("integer", data)
}

func verifyBool(data interface{},
	rv reflect.Value,
	ignore_fields map[string]interface{}) error {

	if _, ok := data.(bool); ok {
		return nil
	}
	return badtype("integer", data)
}

func verifyAnything(data interface{}, rv reflect.Value,
	ignore_fields map[string]interface{}) error {
	return nil
}
