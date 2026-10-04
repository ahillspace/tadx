package value

import (
	"encoding/json"
	"testing"
)

func TestSearchCursorRecordEncodingRemainsStable(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"request", SearchRequest{Types: []string{"workbook"}, Terms: "sales", Limit: 20}, `{"Types":["workbook"],"Terms":"sales","ProjectPath":"","Owner":"","Cursor":"","Limit":20}`},
		{"item", SearchItem{LUID: "wb-1", Type: "workbook", Name: "Sales"}, `{"LUID":"wb-1","Type":"workbook","Name":"Sales","ProjectPath":"","Owner":"","ModifiedAt":""}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(test.value)
			if err != nil || string(got) != test.want {
				t.Fatalf("encoded=%s error=%v want=%s", got, err, test.want)
			}
		})
	}
}
