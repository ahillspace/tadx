package pulse_test

import (
	"reflect"
	"testing"
)

func TestPulsePublishPreservesRepeatedDatasourceMappings(t *testing.T) {
	a := &actions{}
	command := newCommand(a)
	command.SetArgs([]string{"definition", "publish", "--environment", "target", "--workspace", "portable", "--artifact", "artifacts/pulse-definition/example", "--datasource-map", "source,one=destination,one", "--datasource-map", "source-two=destination-two", "--preview"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !a.definitionPublishInput.Preview || !reflect.DeepEqual(a.definitionPublishInput.DatasourceMap, []string{"source,one=destination,one", "source-two=destination-two"}) {
		t.Fatalf("input=%#v", a.definitionPublishInput)
	}
}
