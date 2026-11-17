package importer

import (
	"errors"
	"strings"
	"testing"
)

func collect(input string) (objs []lsObject, invalid int, err error) {
	h := &streamHandler{
		Object:  func(o lsObject) error { objs = append(objs, o); return nil },
		Invalid: func(error) { invalid++ },
	}
	err = streamJSON(strings.NewReader(input), h)
	return
}

func TestStreamJSONFormats(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		ids     []int64
		invalid int
		wantErr bool
	}{
		{"jsonapi document", `{"meta":{"page":{"total":2}},"jsonapi":{"version":"1.0"},"links":{"next":null},"data":[{"type":"orders","id":"1","attributes":{"total":5}},{"type":"orders","id":2,"attributes":{}}]}`, []int64{1, 2}, 0, false},
		{"bare array", `[{"type":"orders","id":"3","attributes":{"a":1}},{"type":"orders","id":"4","attributes":{"a":2}}]`, []int64{3, 4}, 0, false},
		{"ndjson", "{\"type\":\"customers\",\"id\":\"5\",\"attributes\":{\"email\":\"a@b.c\"}}\n{\"type\":\"customers\",\"id\":\"6\",\"attributes\":{}}\n", []int64{5, 6}, 0, false},
		{"concatenated documents", `{"data":[{"id":"7","attributes":{}}]}{"data":[{"id":"8","attributes":{}}]}`, []int64{7, 8}, 0, false},
		{"single resource in data", `{"data":{"type":"stores","id":"9","attributes":{"name":"S"}}}`, []int64{9}, 0, false},
		{"flat objects", `[{"id":10,"name":"flat"},{"id":"11","name":"flat2"}]`, []int64{10, 11}, 0, false},
		{"flat ndjson with variant links array", "{\"id\":12,\"name\":\"v\",\"links\":[{\"title\":\"t\",\"url\":\"u\"}]}\n", []int64{12}, 0, false},
		{"utf-8 bom", "\xef\xbb\xbf" + `[{"id":"13","attributes":{}}]`, []int64{13}, 0, false},
		{"null data", `{"data":null}`, nil, 0, false},
		{"empty data", `{"data":[]}`, nil, 0, false},
		{"empty input", ``, nil, 0, false},
		{"non-object elements", `[1,"a",null,{"id":"14","attributes":{}}]`, []int64{14}, 3, false},
		{"truncated document keeps what was read", `{"data":[{"id":"15","attributes":{}},{"id":"16","attr`, []int64{15}, 0, true},
		{"garbage", `<html>`, nil, 0, true},
		{"attributes of the wrong type", `[{"id":"17","attributes":"x"},{"id":"18","attributes":{}}]`, []int64{18}, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			objs, invalid, err := collect(c.input)
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, c.wantErr)
			}
			var ids []int64
			for _, o := range objs {
				ids = append(ids, o.ID)
			}
			if len(ids) != len(c.ids) {
				t.Fatalf("ids = %v, want %v", ids, c.ids)
			}
			for i := range ids {
				if ids[i] != c.ids[i] {
					t.Fatalf("ids = %v, want %v", ids, c.ids)
				}
			}
			if invalid != c.invalid {
				t.Errorf("invalid = %d, want %d", invalid, c.invalid)
			}
		})
	}
}

func TestStreamJSONFlatObjectKeepsAttributes(t *testing.T) {
	objs, _, err := collect(`{"id":12,"name":"v","links":[{"title":"t","url":"u"}],"total":"7"}`)
	if err != nil || len(objs) != 1 {
		t.Fatal(err, objs)
	}
	a := objs[0].A
	if a.str("name") != "v" || len(a.list("links")) != 1 || a.i64("total") != 7 {
		t.Errorf("attrs = %v", a)
	}
}

func TestStreamHandlerErrorStopsAndIsDistinguishable(t *testing.T) {
	stop := errors.New("stop")
	n := 0
	h := &streamHandler{Object: func(lsObject) error { n++; return stop }}
	err := streamJSON(strings.NewReader(`[{"id":1},{"id":2},{"id":3}]`), h)
	var he *handlerError
	if !errors.As(err, &he) || !errors.Is(err, stop) || n != 1 {
		t.Errorf("err = %v, handler calls = %d", err, n)
	}
}

func TestLimitedReaderRejectsOversizedEntries(t *testing.T) {
	big := `[` + strings.Repeat(`{"id":1},`, 100) + `{"id":2}]`
	h := &streamHandler{Object: func(lsObject) error { return nil }}
	err := streamJSON(&limitedReader{r: strings.NewReader(big), n: 64}, h)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Errorf("err = %v", err)
	}
}

func TestAttrAccessorsAreTolerant(t *testing.T) {
	o, err := parseObject([]byte(`{"type":"x","id":"7","attributes":{"n":"12.6","s":5,"b":"1","z":null,"t":"2024-01-02 03:04:05","u":"1704164645","list":"no","obj":3,"big":12345678901234}}`))
	if err != nil {
		t.Fatal(err)
	}
	a := o.A
	if a.i64("n") != 13 || a.str("s") != "5" || !a.boolean("b") || a.i64p("z") != nil || a.strp("z") != nil || a.boolean("z") || a.i64("missing") != 0 {
		t.Errorf("numbers: %v", a)
	}
	if tm := a.tm("t"); tm == nil || tm.Year() != 2024 || tm.Hour() != 3 {
		t.Errorf("t = %v", tm)
	}
	if tm := a.tm("u"); tm == nil || tm.Unix() != 1704164645 {
		t.Errorf("unix = %v", tm)
	}
	if a.list("list") != nil || a.obj("obj") != nil || a.i64("big") != 12345678901234 {
		t.Errorf("shapes: %v", a)
	}
	if _, err := parseObject([]byte(`[1]`)); err == nil {
		t.Errorf("an array is not an object")
	}
}
