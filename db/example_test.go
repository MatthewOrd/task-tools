package db

import "fmt"

func ExampleValues() {
	s := struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
		Age  int    `db:"age"`
	}{
		1,
		"Albert",
		42,
	}
	fmt.Println(Values(s, "id", "age", "name"))

	// Output:
	// [1 42 Albert]
}

func ExampleCheckColumns() {
	s := struct {
		ID          int    `json:"id" db:"-"`                // omit
		Name        string `json:"name"`                     // column: name
		Age         int    `json:"age"`                      // column: age
		Address     string `json:"address"`                  // column: address
		PhoneNumber string `json:"phone_number" db:"number"` // column: number
	}{
		10,
		"Susan",
		14,
		"123 Main Street",
		"111-222-4567",
	}

	err := CheckColumns(s, "name", "age", "address", "number")
	if err != nil {
		fmt.Println(err)
	}

	err = CheckColumns(s, "id", "phone_number")
	if err != nil {
		fmt.Println(err)
	}

	// Output:
	// columns not found: id, phone_number
}
