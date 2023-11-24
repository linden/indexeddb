//go:build js && wasm

package indexeddb

import "testing"

func TestBasic(t *testing.T) {
	db, err := New("counter", 1, func(up *Upgrade) error {
		up.CreateStore("count")
		up.CreateStore("friends")

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	tx, err := db.NewTransaction([]string{"count"}, ReadWriteMode)
	if err != nil {
		t.Fatal(err)
	}

	str := tx.Store("count")

	err = str.Put("horses", 20)
	if err != nil {
		t.Fatal(err)
	}

	err = str.Put("apples", 10)
	if err != nil {
		t.Fatal(err)
	}

	v, err := str.Get("horses")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%v\n", v.Int())
}
