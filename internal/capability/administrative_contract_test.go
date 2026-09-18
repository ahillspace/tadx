package capability

import (
	"slices"
	"testing"
)

func TestAdministrativeClassificationIsExact(t *testing.T) {
	want := []string{
		"admin.group.create", "admin.group.delete", "admin.group.inspect", "admin.group.list", "admin.group.member.add", "admin.group.member.remove", "admin.group.update",
		"admin.user.create", "admin.user.delete", "admin.user.inspect", "admin.user.list", "admin.user.update",
		"admin.permission.create", "admin.permission.delete", "admin.permission.inspect",
		"admin.label.category.create", "admin.label.category.delete", "admin.label.category.inspect", "admin.label.category.list", "admin.label.category.update",
		"admin.label.value.delete", "admin.label.value.inspect", "admin.label.value.list", "admin.label.value.update",
	}
	var actual []string
	for _, definition := range All() {
		if definition.Administrative {
			actual = append(actual, definition.ID)
		}
	}
	slices.Sort(want)
	if !slices.Equal(actual, want) {
		t.Fatalf("administrative capabilities=%v, want=%v", actual, want)
	}
	for _, id := range []string{"project.delete", "workbook.update", "datasource.update", "flow.update", "job.cancel", "content.label.update"} {
		definition, ok := Lookup(id)
		if !ok || definition.Administrative {
			t.Errorf("%s incorrectly classified", id)
		}
	}
}
