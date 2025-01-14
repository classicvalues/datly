package data

import (
	"context"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/viant/assertly"
	"github.com/viant/dsunit"
	"github.com/viant/toolbox"
	"path"
	"testing"
)

func TestNewResourceFromURL(t *testing.T) {
	testLocation := toolbox.CallerDirectory(3)

	testCases := []struct {
		description string
		url         string
		expect      string
	}{
		{
			url: "case001",
			expect: `{
	"Connectors": [
		{
			"DSN": "./testdata/db/mydb.db",
			"Driver": "sqlite3",
			"Name": "mydb"
		}
	],
	"Views": [
		{
			"Alias": "t",
			"BatchReadSize": null,
			"CaseFormat": "lu",
			"Caser": 5,
			"Columns": [
				{
					"DataType": "Int",
					"Filterable": false,
					"Name": "id"
				},
				{
					"DataType": "Float",
					"Filterable": false,
					"Name": "quantity"
				},
				{
					"DataType": "Int",
					"Filterable": false,
					"Name": "event_type_id"
				}
			],
			"Connector": {
				"DSN": "./testdata/db/mydb.db",
				"Driver": "sqlite3",
				"Name": "mydb",
				"Ref": "mydb"
			},
			"MatchStrategy": "read_matched",
			"Name": "events",
			"Schema": {
				"Name": "events",
				"OmitEmpty": false
			},
			"SelectorConstraints": {
				"Columns": null,
				"Criteria": null,
				"Limit": null,
				"Offset": null,
				"OrderBy": null
			},
			"Table": "events"
		}
	]
}`,
		},
	}

	for _, testCase := range testCases {
		if !dsunit.InitFromURL(t, path.Join(testLocation, "testdata/config.yaml")) {
			return
		}

		resource, err := NewResourceFromURL(context.TODO(), path.Join(testLocation, "testdata", testCase.url, "resource.yaml"), Types{})
		if !assert.Nil(t, err, testCase.description) {
			continue
		}

		if !assertly.AssertValues(t, testCase.expect, resource, testCase.description) {
			toolbox.DumpIndent(resource, true)
		}
	}
}
