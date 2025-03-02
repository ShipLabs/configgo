package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// var (
// 	Example = map[string]any{
// 		"name": "exampleName",
// 		"sample": map[string]any{
// 			"test": "nestedTestValue",
// 		},
// 	}
// )

var (
	ErrNotPointer           = errors.New("parameter is not a pointer")
	ErrNotStruct            = errors.New("parameter should be a pointer to a struct")
	ErrRequiredFieldMissing = errors.New("required field missing")
)

func ErrTypeNotConvertible(val any, taip string) error {
	return fmt.Errorf("cannot convert to " + taip + ": " + fmt.Sprintf("%v", val))
}

func ErrUnsupportedFieldType(field reflect.Value, defaultField bool) error {
	message := "unsupported field type"
	if defaultField {
		message += "for default value"
	}
	return fmt.Errorf(message + ": " + field.Type().String())
}

func StructMapper(data map[string]any, config any) error {
	configVal := reflect.ValueOf(config)
	if configVal.Kind() != reflect.Pointer {
		return ErrNotPointer
	}

	derefVal := configVal.Elem()
	if derefVal.Kind() != reflect.Struct {
		return ErrNotStruct
	}

	types := derefVal.Type()
	for i := range derefVal.NumField() {
		field, fieldType := derefVal.Field(i), types.Field(i)
		if !field.CanSet() {
			continue
		}

		fieldName, tagSlice := fieldType.Name, strings.Split(fieldType.Tag.Get("configgo"), ",")
		nameField := tagSlice[0]
		if nameField != "" {
			fieldName = nameField
		}

		//if field is a struct we want to recursively call mapper on that field
		//this assumes that the field is a nested struct and not a group of individual fields identified using dot notation
		//example: this currently works for map[string]any{	"name": "exampleName","sample": map[string]any{	"test": "nestedTestValue",}
		//it does not work for map[string]any{	"name": "exampleName","sample.test": "nestedValue"} where sample is a struct and test is a field in the struct
		//it also does not work for a combination of both such fields
		//will possibly implement later depending on how the final config loading works
		if field.Kind() == reflect.Struct {
			nestedData, ok := data[fieldName].(map[string]any)
			if ok && len(nestedData) > 0 {
				err := StructMapper(nestedData, field.Addr().Interface())
				if err != nil {
					return err
				}
			}
			continue
		}

		//if the field is not a struct
		dataVal, ok := data[fieldName]
		if !ok {
			defaultVal := checkDefault(tagSlice)
			if defaultVal != "" {
				if err := setFieldFromString(field, defaultVal); err != nil {
					return err
				}
			}

			isRequired := isFieldRequired(tagSlice)
			if isRequired {
				return ErrRequiredFieldMissing
			}
			continue
		}

		if err := setField(field, dataVal); err != nil {
			return err
		}
	}

	return nil
}

func setFieldFromString(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(intVal)
	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(boolVal)
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(floatVal)
	default:
		return ErrUnsupportedFieldType(field, true)

	}
	return nil
}

func setField(field reflect.Value, value any) error {
	switch field.Kind() {
	case reflect.String:
		str, ok := value.(string)
		if !ok {
			str = fmt.Sprintf("%v", value)
		}
		field.SetString(str)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var val int64
		switch v := value.(type) {
		case int:
			val = int64(v)
		case int64:
			val = v
		case string:
			intVal, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return err
			}
			val = intVal
		default:
			return ErrTypeNotConvertible(value, "int")
		}
		field.SetInt(val)

	case reflect.Bool:
		var val bool
		switch v := value.(type) {
		case bool:
			val = v
		case string:
			boolVal, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			val = boolVal
		default:
			return ErrTypeNotConvertible(value, "bool")
		}
		field.SetBool(val)

	case reflect.Float32, reflect.Float64:
		var val float64
		switch v := value.(type) {
		case float64:
			val = float64(v)
		case int:
			val = float64(v)
		case string:
			floatVal, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			val = floatVal
		default:
			return ErrTypeNotConvertible(value, "float")
		}
		field.SetFloat(val)
	default:
		return ErrUnsupportedFieldType(field, false)

	}

	return nil
}

func isFieldRequired(tagSlice []string) bool {
	return slices.Contains(tagSlice, "required")
}

func checkDefault(tagSlice []string) string {
	for _, tag := range tagSlice {
		if strings.HasPrefix(tag, "default=") {
			return strings.TrimPrefix(tag, "default=")
		}
	}
	return ""
}

func BasicLoad(path string) (config map[string]any, err error) {
	return LoadSingleFile(path)
}

func LoadIntoStruct(path string, configPtr any) (err error) {
	data, err := LoadSingleFile(path)
	if err != nil {
		return
	}

	return StructMapper(data, configPtr)
}

// basic file loading
func LoadSingleFile(path string) (config map[string]any, err error) {
	config = make(map[string]any)
	//for now handle .env files alone (other file extensions to consider inclue )
	if path == "" {
		path = ".env"
	}
	file, err := os.Open(path)
	if err != nil {
		return //was tempted to use log.fatal but decided against it, let the caller decide how they want to handle errors in the event of a failed config load
	}
	defer file.Close()

	scannner := bufio.NewScanner(file)
	for scannner.Scan() {
		skip, key, val := parseLine(scannner.Text())
		if skip {
			continue
		}
		config[key] = val
	}

	return
}

func parseLine(line string) (skip bool, key string, val string) {
	skip = true
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	const INVALID_LINE_PREFIXES = "//!@#;:=[]"
	for _, prefix := range INVALID_LINE_PREFIXES {
		if strings.HasPrefix(line, string(prefix)) {
			return
		}
	}

	//test this againts edge cases(":=")
	keyVal := strings.FieldsFunc(line, func(r rune) bool {
		return r == '=' || r == ':'
	})
	if len(keyVal) != 2 {
		return
	}

	//checks if the line contains section markers like: [section]
	if strings.HasPrefix(keyVal[0], "[") && strings.HasSuffix(keyVal[1], "]") {
		return
	}

	return false, keyVal[0], keyVal[1]

}

// for testing
func main() {
	// config, _ := LoadSingleFile("")
	// type nest struct {
	// 	Test string `configgo:"test"`
	// }
	// type eg struct {
	// 	Name   string `configgo:"name,required,default=sample"`
	// 	Sample nest   `configgo:"sample"`
	// }

	// instance := eg{}
	// fmt.Println(Example)
	// err := StructMapper(Example, &instance)
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	// fmt.Println(instance, "config")
}
