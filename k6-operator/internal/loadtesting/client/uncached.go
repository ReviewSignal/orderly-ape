package client

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"k8s.io/apimachinery/pkg/conversion"

	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
	"github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/runtime"
)

// UncachedClient represents an instance of Client, without an internal cache.
type UncachedClient struct {
	client *resty.Client

	options
}

// NewUncachedClient instantiate an uncached client.
func NewUncachedClient(opts ...Option) (*UncachedClient, error) {
	options := defaultOptions()

	// Apply options.
	for _, opt := range opts {
		opt(options)
	}

	c := resty.New().SetDisableWarn(true)
	c = c.SetBaseURL(options.URL).SetBasicAuth(options.Username, options.Password)

	return &UncachedClient{
		client:  c,
		options: *options,
	}, nil
}

// List retrieve a list of objects from the remote and store them in obj.
func (c *UncachedClient) List(ctx context.Context, obj runtime.ObjectList) error {
	endpoint, err := runtime.Schema.GetEndpointForList(obj)
	if err != nil {
		return err
	}

	sl := reflect.SliceOf(runtime.Schema.GetObjType(obj))
	store := reflect.MakeSlice(sl, 0, 0).Interface()

	resp, respErr := c.client.R().
		SetResult(store).
		SetPathParams(map[string]string{"locationName": c.Region}).
		SetContext(ctx).
		Get(endpoint)

	if respErr != nil {
		return respErr
	}

	if resp.IsError() {
		return NewStatusError(resp)
	}

	result := resp.Result()
	resultReflect := reflect.ValueOf(result)

	// Check if v is a pointer and points to a slice
	if resultReflect.Kind() != reflect.Ptr || resultReflect.Elem().Kind() != reflect.Slice {
		return errors.New("Result is not a slice")
	}

	// Now you can iterate over the slice
	slice := resultReflect.Elem()
	items := make([]runtime.Object, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		ok := false
		items[i], ok = slice.Index(i).Addr().Interface().(runtime.Object)
		if !ok {
			return errors.New("result is not of type Object")
		}
	}

	obj.SetItems(items)
	return nil
}

// Get retrieves an object by it's name, from the remote.
func (c *UncachedClient) Get(ctx context.Context, id string, obj runtime.Object) error {
	_, err := conversion.EnforcePtr(obj)
	if err != nil {
		return err
	}

	endpoint, err := runtime.Schema.GetEndpointForObj(obj)
	if err != nil {
		return err
	}

	if strings.Contains(endpoint, "%s") {
		endpoint = fmt.Sprintf(endpoint, id)
	}

	realType := reflect.Indirect(reflect.ValueOf(obj))
	resp, respErr := c.client.R().
		SetResult(realType.Interface()).
		SetPathParams(map[string]string{"locationName": c.Region}).
		SetContext(ctx).
		Get(endpoint)

	if respErr != nil {
		return respErr
	}

	if resp.IsError() {
		return NewStatusError(resp)
	}

	newObj := reflect.Indirect(reflect.ValueOf(resp.Result()))
	outVal := reflect.ValueOf(obj)
	reflect.Indirect(outVal).Set(newObj)

	return nil
}

func (c *UncachedClient) Watch(obj runtime.Object) (<-chan runtime.Object, error) {
	ch := make(chan runtime.Object)

	c.Logger.Info("start watching", "type", runtime.RealTypeOf(obj))

	go func() {
		t := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-c.StopCh:
				return
			case <-t.C:
				objList := api.JobList{}

				if err := c.List(context.Background(), &objList); err != nil {
					c.Logger.Error(err, "can't list objects")
					time.Sleep(8 * time.Second)
				}

				for idx := range objList.Items {
					ch <- &objList.Items[idx]
				}
			}
		}
	}()

	return ch, nil
}

func (c *UncachedClient) Create(ctx context.Context, obj runtime.Object) error {
	_, err := conversion.EnforcePtr(obj)
	if err != nil {
		return err
	}

	endpoint, err := runtime.Schema.GetEndpointForList(obj)
	if err != nil {
		return err
	}

	realType := reflect.Indirect(reflect.ValueOf(obj))
	resp, respErr := c.client.R().
		SetResult(realType.Interface()).
		SetBody(obj).
		SetPathParams(map[string]string{"locationName": c.Region}).
		SetContext(ctx).
		Post(endpoint)
	if respErr != nil {
		return respErr
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 201 && resp.StatusCode() != 202 && resp.StatusCode() != 204 {
		return NewStatusError(resp)
	}

	newObj := reflect.Indirect(reflect.ValueOf(resp.Result()))
	outVal := reflect.ValueOf(obj)
	reflect.Indirect(outVal).Set(newObj)
	return nil
}

func (c *UncachedClient) Update(ctx context.Context, obj runtime.Object) error {
	_, err := conversion.EnforcePtr(obj)
	if err != nil {
		return err
	}
	endpoint, err := runtime.Schema.GetEndpointForObj(obj)
	if err != nil {
		return err
	}

	if strings.Contains(endpoint, "%s") {
		endpoint = fmt.Sprintf(endpoint, obj.GetName())
	}

	realType := reflect.Indirect(reflect.ValueOf(obj))
	resp, respErr := c.client.R().
		SetResult(realType.Interface()).
		SetBody(obj).
		SetPathParams(map[string]string{"locationName": c.Region}).
		SetContext(ctx).
		Put(endpoint)
	if respErr != nil {
		return respErr
	}

	if resp.StatusCode() != 200 {
		return NewStatusError(resp)
	}

	newObj := reflect.Indirect(reflect.ValueOf(resp.Result()))
	outVal := reflect.ValueOf(obj)
	reflect.Indirect(outVal).Set(newObj)
	return nil
}
