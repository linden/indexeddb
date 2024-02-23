//go:build js && wasm

package indexeddb

import "testing"

func TestBasic(t *testing.T) {
	db, err := New("counter", 1, func(up *Upgrade) error {
		up.CreateStore("count")

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

	if v.Int() != 20 {
		t.Fatalf("expected 20 but got %d", v.Int())
	}
}

func TestIndex(t *testing.T) {
	db, err := New("index", 1, func(up *Upgrade) error {
		str := up.NewStore("people", &StoreConfig{
			KeyPath: "age",
		})
		str.NewIndex("age")
		str.NewIndex("name")

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	tx, err := db.NewTransaction([]string{"people"}, ReadWriteMode)
	if err != nil {
		t.Fatal(err)
	}

	str := tx.Store("people")

	obj := Object.New()
	obj.Set("age", 25)
	obj.Set("name", "jim")

	err = str.Add(nil, obj)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("get by age", func(t *testing.T) {
		jim, err := str.Index("age").Get(25)
		if err != nil {
			t.Fatal(err)
		}

		nm := jim.Get("name").String()

		if nm != "jim" {
			t.Fatalf("expected jim got %s", nm)
		}
	})

	t.Run("get by name", func(t *testing.T) {
		jim, err := str.Index("name").Get("jim")
		if err != nil {
			t.Fatal(err)
		}

		age := jim.Get("age").Int()

		if age != 25 {
			t.Fatalf("expected 25 got %d", age)
		}
	})
}
