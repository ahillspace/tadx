package app

import (
	labelcategory "github.com/ahillspace/tadx/actions/admin/labelcategory"
	labelvalue "github.com/ahillspace/tadx/actions/admin/labelvalue"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
)

func adminLabelDependencies(runtime *runtimeDependencies) *admincli.LabelDependencies {
	values := labelvalue.New(adminLabelValueProvider{runtime: runtime})
	categories := labelcategory.New(adminLabelCategoryProvider{runtime: runtime})
	return &admincli.LabelDependencies{ValueLister: values, ValueInspector: values, ValueUpdater: values, ValueDeleter: values, CategoryLister: categories, CategoryInspector: categories, CategoryCreator: categories, CategoryUpdater: categories, CategoryDeleter: categories}
}
