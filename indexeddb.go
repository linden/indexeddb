//go:build js && wasm

package indexeddb

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"syscall/js"
)

var (
	IndexedDB = js.Global().Get("indexedDB")
	Array     = js.Global().Get("Array")
)

var (
	ErrValueNotFound = errors.New("value not found")
	ErrKeyInvalid    = errors.New("key is invalid")
	ErrValueInvalid  = errors.New("value is invalid")
	ErrInvalidType   = errors.New("type is not accepted")
)

var Logger *slog.Logger

func init() {
	// discard logs by default.
	if Logger == nil {
		Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

type Store struct {
	value js.Value
}

// keys and values can be pretty much anything in indexeddb.
// we limit to strings, bools, ints, uints, floats and slices.
//
// values indexedb supports: https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API/Basic_Terminology.
// values Go supports: https://github.com/golang/go/blob/676002986c55a296ea348c30706d6b63a3256b7f/src/syscall/js/js.go#L152-L211.
func valid(x any) error {
	switch v := reflect.ValueOf(x); {
	// check if the value is a string, bool, int, uint or float.
	case v.Kind() == reflect.String, v.Kind() == reflect.Bool, v.CanInt(), v.CanUint(), v.CanFloat():
		return nil

	default:
		return errors.Join(ErrInvalidType, fmt.Errorf("type: %T", x))
	}
}

// put is either an insert or an update,
func (s *Store) Put(key any, value any) error {
	Logger.Debug("store put", "key", key, "value", value)

	err := valid(key)
	if err != nil {
		return errors.Join(ErrKeyInvalid, err)
	}

	err = valid(value)
	if err != nil {
		return errors.Join(ErrValueInvalid, err)
	}

	// put the key and value.
	// the key is the 2nd argument as it's technically optional.
	req := s.value.Call("put", value, key)

	// wait for the request to complete.
	return await(req, nil)
}

// get is a query for the key.
func (s *Store) Get(key any) (*js.Value, error) {
	Logger.Debug("store get", "key", key)

	err := valid(key)
	if err != nil {
		return nil, errors.Join(ErrKeyInvalid, err)
	}

	req := s.value.Call("get", key)

	// wait for the request to complete.
	err = await(req, nil)
	if err != nil {
		return nil, err
	}

	res := req.Get("result")

	// check if the result was not found.
	if res.IsUndefined() {
		return nil, ErrValueNotFound
	}

	// return the result.
	return &res, nil
}

func (s *Store) Delete(key any) error {
	err := valid(key)
	if err != nil {
		return errors.Join(ErrKeyInvalid, err)
	}

	// make the request to delete the key.
	req := s.value.Call("delete", key)

	// wait for the request to complete.
	return await(req, nil)
}

func (s *Store) Clear() error {
	// make the request to clear.
	req := s.value.Call("clear")

	// wait for the request to complete.
	return await(req, nil)
}

func (s *Store) Count() (int, error) {
	req := s.value.Call("count")

	err := await(req, nil)
	if err != nil {
		return 0, err
	}

	return req.Get("result").Int(), nil
}

// an upgrade is a database connection before needing it's objects/indexes established.
type Upgrade struct {
	value js.Value
}

func (up *Upgrade) CreateStore(name string) {
	up.value.Call("createObjectStore", name)
}

type Mode int

const (
	ReadMode Mode = iota
	ReadWriteMode
)

var modes = [...]string{
	ReadMode:      "readonly",
	ReadWriteMode: "readwrite",
}

func (m Mode) Verify() bool {
	return m == ReadMode || m == ReadWriteMode
}

func (m Mode) String() string {
	return modes[int(m)]
}

// https://developer.mozilla.org/en-US/docs/Web/API/IDBTransaction.
type Transaction struct {
	value js.Value
}

func (tx *Transaction) Store(name string) *Store {
	// get the store.
	val := tx.value.Call("objectStore", name)

	return &Store{
		value: val,
	}
}

// https://developer.mozilla.org/en-US/docs/Web/API/IDBDatabase.
type DB struct {
	value js.Value
}

func (db *DB) NewTransaction(stores []string, mode Mode) (*Transaction, error) {
	// ensure we have at least 1 store.
	if len(stores) == 0 {
		return nil, errors.New("at least 1 store must be requested")
	}

	// ensure the mode if valid.
	if !mode.Verify() {
		return nil, errors.New("mode must be read or read write")
	}

	// create a new javascript array.
	strs := Array.New()

	// HACK: create a javascript array of strings from our Go slice of strings
	// Go does not do this by default.
	for _, str := range stores {
		// append to the javascript array.
		strs.Call("push", str)
	}

	// create the transaction.
	val := db.value.Call("transaction", strs, mode.String())

	// handle the error event.
	listen(val, "onerror", func(v js.Value) {
		// wrap and return the error event.
		panic(wrapError(v))
	})

	return &Transaction{
		value: val,
	}, nil
}

// close the database.
func (db *DB) Close() error {
	db.value.Call("close")

	return nil
}

func New(name string, version int, upgrade func(up *Upgrade) error) (*DB, error) {
	errChan := make(chan error, 1)

	// open the database.
	req := IndexedDB.Call("open", name, version)

	// handle the upgrade event.
	listen(req, "onupgradeneeded", func(v js.Value) {
		// get the database connection.
		val := v.Get("target").Get("result")

		// create a upgrade.
		up := &Upgrade{
			value: val,
		}

		// call the upgrade event.
		err := upgrade(up)
		if err != nil {
			errChan <- err
		}
	})

	err := await(req, errChan)
	if err != nil {
		return nil, err
	}

	// return the database connection.
	return &DB{
		value: req.Get("result"),
	}, nil
}

// wait for a `IDBRequest` to either return and error or success message.
// optionally pass an error channel.
func await(v js.Value, errChan chan error) error {
	if errChan == nil {
		errChan = make(chan error, 1)
	}

	// handle the error event.
	listen(v, "onerror", func(v js.Value) {
		// wrap and return the error event.
		errChan <- wrapError(v)
	})

	// handle the success event.
	listen(v, "onsuccess", func(v js.Value) {
		errChan <- nil
	})

	// wait for either the error or success message.
	// TODO: add timeout.
	return <-errChan
}

// listen for an event.
func listen(v js.Value, target string, fn func(event js.Value)) {
	var h js.Func

	// create the handler.
	h = js.FuncOf(func(this js.Value, args []js.Value) any {
		// forward the event argument.
		fn(args[0])

		// remove the function.
		h.Release()

		// return nothing.
		return nil
	})

	// set the handler.
	v.Set(target, h)
}

func wrapError(v js.Value) error {
	// ensure we have method to convert to a string,
	if v.Get("toString").IsNull() {
		return errors.New("invalid javascript error")
	}

	// convert the error to an error.
	return errors.New(v.Call("toString").String())
}