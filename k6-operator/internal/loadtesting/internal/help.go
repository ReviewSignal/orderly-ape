package internal

import (
	"errors"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/conversion"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

var (
	errExpectFieldItems = errors.New("no Items field in this object")
	errExpectSliceItems = errors.New("items field must be a slice of objects")

	// objectSliceType is the type of a slice of Objects
	objectSliceType = reflect.TypeOf([]runtime.Object{})
)

// GetItemsPtr returns a pointer to the list object's Items member.
// If 'list' doesn't have an Items member, it's not really a list type
// and an error will be returned.
// This function will either return a pointer to a slice, or an error, but not both.
func GetItemsPtr(list runtime.Object) (interface{}, error) {
	obj, err := getItemsPtr(list)
	if err != nil {
		return nil, fmt.Errorf("%T is not a list: %v", list, err)
	}

	return obj, nil
}

// getItemsPtr returns a pointer to the list object's Items member or an error.
func getItemsPtr(list runtime.Object) (interface{}, error) {
	v, err := conversion.EnforcePtr(list)
	if err != nil {
		return nil, err
	}

	items := v.FieldByName("Items")
	if !items.IsValid() {
		return nil, errExpectFieldItems
	}

	switch items.Kind() {
	case reflect.Interface, reflect.Ptr:
		target := reflect.TypeOf(items.Interface()).Elem()
		if target.Kind() != reflect.Slice {
			return nil, errExpectSliceItems
		}

		return items.Interface(), nil
	case reflect.Slice:
		return items.Addr().Interface(), nil
	default:
		return nil, errExpectSliceItems
	}
}

// SetList sets the given list object's Items member have the elements given in
// objects.
// Returns an error if list is not a List type (does not have an Items member),
// or if any of the objects are not of the right type.
func SetList(list runtime.Object, objects []runtime.Object) error {
	// list need to have an Items []<type that implements runtime.Object>
	itemsPtr, err := GetItemsPtr(list)
	if err != nil {
		return err
	}

	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return err
	}

	// if the items is a slice, we just need to set it's value
	if items.Type() == objectSliceType {
		items.Set(reflect.ValueOf(objects))
		return nil
	}

	// otherwise, we'll need to create and manually set the value for each element
	slice := reflect.MakeSlice(items.Type(), len(objects), len(objects))

	for i := range objects {
		dest := slice.Index(i)

		// check to see if you're directly assignable
		if reflect.TypeOf(objects[i]).AssignableTo(dest.Type()) {
			dest.Set(reflect.ValueOf(objects[i]))
			continue
		}

		// otherwise, get the value of the current object's dereferenced pointer
		src, err := conversion.EnforcePtr(objects[i])
		if err != nil {
			return err
		}

		// check if we can assign directly, or we need to convert it first
		switch kind := src.Type(); {
		case kind.AssignableTo(dest.Type()):
			dest.Set(src)
		case kind.ConvertibleTo(dest.Type()):
			dest.Set(src.Convert(dest.Type()))
		default:
			return fmt.Errorf("item[%d]: can't assign or convert %v into %v", i, src.Type(), dest.Type())
		}
	}

	items.Set(slice)

	return nil
}

// ExtractList returns obj's Items element as an array of runtime.Objects.
// Returns an error if obj is not a List type (does not have an Items member).
func ExtractList(obj runtime.Object) ([]runtime.Object, error) {
	itemsPtr, err := GetItemsPtr(obj)
	if err != nil {
		return nil, err
	}

	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return nil, err
	}

	list := make([]runtime.Object, items.Len())
	for i := range list {
		raw := items.Index(i)
		switch item := raw.Interface().(type) {
		case runtime.Object:
			list[i] = item
		default:
			var found bool
			if list[i], found = raw.Addr().Interface().(runtime.Object); !found {
				return nil, fmt.Errorf("%v: item[%v]: Expected object, got %#v(%s)", obj, i, raw.Interface(), raw.Kind())
			}
		}
	}

	return list, nil
}
