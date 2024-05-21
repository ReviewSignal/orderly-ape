package runtime

import (
	"fmt"
	"reflect"
	"sync"
)

type SchemeRecord struct {
	TypeName     string
	ListTypeName string
	Endpoint     string
	ListEndpoint string
	ListType     reflect.Type
	ObjType      reflect.Type
}

// Scheme is a store that maps a certain resource type to it's concrete type and it's HTTP endpoints.
type Scheme struct {
	mu           sync.RWMutex
	objToRecord  map[string]*SchemeRecord
	typeToRecord map[reflect.Type]*SchemeRecord
}

// NewScheme instantiate a scheme.
func NewScheme() *Scheme {
	return &Scheme{
		mu:           sync.RWMutex{},
		objToRecord:  map[string]*SchemeRecord{},
		typeToRecord: map[reflect.Type]*SchemeRecord{},
	}
}

var missingObjTypeFmt = "missing type %s from scheme"

func RealTypeOf(obj interface{}) reflect.Type {
	if reflect.ValueOf(obj).Kind() == reflect.Ptr {
		return reflect.Indirect(reflect.ValueOf(obj)).Type()
	}

	return reflect.TypeOf(obj)
}

// Register takes an object like resource and adds it to the scheme.
func (s *Scheme) Register(obj Object, list ObjectList, endpoint string) {
	typeObj := RealTypeOf(obj)
	listTypeObj := RealTypeOf(list)

	r := &SchemeRecord{
		TypeName:     typeObj.String(),
		ListTypeName: listTypeObj.String(),
		Endpoint:     endpoint + "/%s",
		ListEndpoint: endpoint,
		ObjType:      typeObj,
		ListType:     listTypeObj,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.objToRecord[r.TypeName] = r
	s.objToRecord[r.ListTypeName] = r
	s.typeToRecord[r.ObjType] = r
	s.typeToRecord[r.ListType] = r

}

// NewObj is a helper method that dynamically creates a single object for a registered type or type list.
func (s *Scheme) NewObj(kind string) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.objToRecord[kind]
	if !exists {
		return nil, fmt.Errorf(missingObjTypeFmt, kind)
	}
	reflectType := record.ObjType

	obj, ok := (reflect.New(reflectType).Interface()).(Object)
	if !ok {
		return nil, fmt.Errorf("%s doesn't implement interface Object", reflectType)
	}

	return obj, nil
}

// GetEndpointForObj returns the registered endpoint for a single object.
func (s *Scheme) GetEndpointForObj(obj interface{}) (string, error) {
	typeName := RealTypeOf(obj).String()

	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.objToRecord[typeName]
	if !exists {
		return "", fmt.Errorf(missingObjTypeFmt, typeName)
	}

	return record.Endpoint, nil
}

// GetEndpointForList returns the registered endpoint for a list of objects.
func (s *Scheme) GetEndpointForList(obj interface{}) (string, error) {
	typeName := RealTypeOf(obj).String()

	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.objToRecord[typeName]
	if !exists {
		return "", fmt.Errorf(missingObjTypeFmt, typeName)
	}

	return record.ListEndpoint, nil
}

func (s *Scheme) GetObjType(obj interface{}) reflect.Type {
	objType := RealTypeOf(obj)

	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.typeToRecord[objType]
	if !exists {
		panic(fmt.Errorf(missingObjTypeFmt, objType))
	}

	return record.ObjType
}

// Schema is a global store for all the registered types.
var Schema = NewScheme()
