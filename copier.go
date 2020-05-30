package copier

import (
	"database/sql"
	"errors"
	"reflect"
)

var (
	jsonTagTo, jsonTagFrom string
)

//Copy copies an interface (and its contents) into another interface where it can.
//It copies only where the json tags match. Field names are ignored
func Copy(toValue interface{}, fromValue interface{}) (err error) {
	var (
		isSlice bool
		amount  = 1
		from    = indirect(reflect.ValueOf(fromValue))
		to      = indirect(reflect.ValueOf(toValue))
	)

	if !to.CanAddr() {
		return errors.New("copy to value is unaddressable")
	}

	if !from.IsValid() {
		return
	}

	fromType := indirectType(from.Type())
	toType := indirectType(to.Type())

	jsonTagTo = string(to.Type().Field(0).Tag.Get("json"))
	jsonTagFrom = string(from.Type().Field(0).Tag.Get("json"))

	if fromType.Kind() != reflect.Struct && from.Type().AssignableTo(to.Type()) && (jsonTagFrom == jsonTagTo) {
		to.Set(from)
		return
	}

	// if neither are structs return
	if fromType.Kind() != reflect.Struct || toType.Kind() != reflect.Struct {
		return
	}

	if to.Kind() == reflect.Slice {
		isSlice = true
		if from.Kind() == reflect.Slice {
			amount = from.Len()
		}
	}

	for i := 0; i < amount; i++ {
		var dest, source reflect.Value

		if isSlice {
			if from.Kind() == reflect.Slice {
				source = indirect(from.Index(i))
			} else {
				source = indirect(from)
			}
			dest = indirect(reflect.New(toType).Elem())
		} else {
			source = indirect(from)
			dest = indirect(to)
		}

		if source.IsValid() {
			fromTypeFields := deepFields(fromType)

			for _, field := range fromTypeFields {

				name := field.Name

				//get tag JSON of field
				fieldJSONTag := string(field.Tag.Get("json"))

				if fromField := fieldByJSONTag(source, fieldJSONTag); fromField.IsValid() {
					//if fromField := source.FieldByName(name); fromField.IsValid() {
					if toField := fieldByJSONTag(source, fieldJSONTag); toField.IsValid() {
						//if toField := dest.FieldByName(name); toField.IsValid() {
						if toField.CanSet() {
							if !set(toField, fromField) {
								if err := Copy(toField.Addr().Interface(), fromField.Interface()); err != nil {
									return err
								}
							}
						}
					} else {
						// try to set method
						var toMethod reflect.Value
						if dest.CanAddr() {
							toMethod = dest.Addr().MethodByName(name)
						} else {
							toMethod = dest.MethodByName(name)
						}

						if toMethod.IsValid() && toMethod.Type().NumIn() == 1 && fromField.Type().AssignableTo(toMethod.Type().In(0)) {
							toMethod.Call([]reflect.Value{fromField})
						}
					}
				}
			}

			// Copy from method to field
			for _, field := range deepFields(toType) {
				name := field.Name
				fieldJSONTag := string(field.Tag.Get("json"))

				var fromMethod reflect.Value
				if source.CanAddr() {
					//fromMethod = source.Addr().MethodByName(name)
					fromMethod = methodByJSONTag(source.Addr(), fieldJSONTag)
				} else {
					//fromMethod = source.MethodByName(name)
					fromMethod = methodByJSONTag(source, fieldJSONTag)
				}

				if fromMethod.IsValid() && fromMethod.Type().NumIn() == 0 && fromMethod.Type().NumOut() == 1 {
					//if toField := dest.FieldByName(name); toField.IsValid() && toField.CanSet() {
					if toField := fieldByJSONTag(dest, name); toField.IsValid() && toField.CanSet() {
						values := fromMethod.Call([]reflect.Value{})
						if len(values) >= 1 {
							set(toField, values[0])
						}
					}
				}
			}
		}
		if isSlice {
			//todo: check if json tags match

			if dest.Addr().Type().AssignableTo(to.Type().Elem()) {
				to.Set(reflect.Append(to, dest.Addr()))
			} else if dest.Type().AssignableTo(to.Type().Elem()) {
				to.Set(reflect.Append(to, dest))
			}
		}
	}
	return
}

func methodByJSONTag(source reflect.Value, jsonTag string) reflect.Value {
	//todo: test

	method := reflect.Value{}
	reflectType := indirectType(source.Type())
	if reflectType.Kind() != reflect.Struct {
		return method
	}

	for i := 0; i < reflectType.NumMethod(); i++ {
		method = source.Method(i)
		methodJSONTag := string(method.Type().Field(0).Tag.Get("json"))
		if methodJSONTag == jsonTag {
			return method
		}
	}
	return method
}

func fieldByJSONTag(source reflect.Value, jsonTag string) reflect.Value {
	field := reflect.Value{}
	reflectType := indirectType(source.Type())
	if reflectType.Kind() != reflect.Struct {
		return field //, errors.New("source is not a struct")
	}

	for i := 0; i < reflectType.NumField(); i++ {
		field = source.Field(i)
		fieldJSONTag := string(field.Type().Field(0).Tag.Get("json"))
		if fieldJSONTag == jsonTag {
			return field
		}
	}
	return field //, errors.New("source does not contain given json tag")
}

func deepFields(reflectType reflect.Type) []reflect.StructField {
	var fields []reflect.StructField

	if reflectType = indirectType(reflectType); reflectType.Kind() == reflect.Struct {
		for i := 0; i < reflectType.NumField(); i++ {
			v := reflectType.Field(i)
			if v.Anonymous {
				fields = append(fields, deepFields(v.Type)...)
			} else {
				fields = append(fields, v)
			}
		}
	}

	return fields
}

func indirect(reflectValue reflect.Value) reflect.Value {
	for reflectValue.Kind() == reflect.Ptr {
		reflectValue = reflectValue.Elem()
	}
	return reflectValue
}

func indirectType(reflectType reflect.Type) reflect.Type {
	for reflectType.Kind() == reflect.Ptr || reflectType.Kind() == reflect.Slice {
		reflectType = reflectType.Elem()
	}
	return reflectType
}

func set(to, from reflect.Value) bool {

	if jsonTagFrom != jsonTagTo {
		return false
	}

	if from.IsValid() {
		if to.Kind() == reflect.Ptr {
			//set `to` to nil if from is nil
			if from.Kind() == reflect.Ptr && from.IsNil() {
				to.Set(reflect.Zero(to.Type()))
				return true
			} else if to.IsNil() {
				to.Set(reflect.New(to.Type().Elem()))
			}
			to = to.Elem()
		}

		if from.Type().ConvertibleTo(to.Type()) {
			to.Set(from.Convert(to.Type()))
		} else if scanner, ok := to.Addr().Interface().(sql.Scanner); ok {
			err := scanner.Scan(from.Interface())
			if err != nil {
				return false
			}
		} else if from.Kind() == reflect.Ptr {
			return set(to, from.Elem())
		} else {
			return false
		}
	}
	return true
}
