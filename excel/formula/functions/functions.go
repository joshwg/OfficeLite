package functions

import (
	"sync"

	"officelite/excel/formula"
	"officelite/excel/formula/functions/database"
	"officelite/excel/formula/functions/datetime"
	"officelite/excel/formula/functions/engineering"
	"officelite/excel/formula/functions/financial"
	"officelite/excel/formula/functions/info"
	"officelite/excel/formula/functions/logical"
	"officelite/excel/formula/functions/lookup"
	"officelite/excel/formula/functions/mathtrig"
	"officelite/excel/formula/functions/text"
)

var (
	once     sync.Once
	registry *formula.Registry
)

// Default returns the registry with every implemented worksheet function.
// It is built once and must not be modified.
func Default() *formula.Registry {
	once.Do(func() {
		registry = formula.NewRegistry()
		for _, register := range []func(*formula.Registry){
			database.Register,
			datetime.Register,
			engineering.Register,
			financial.Register,
			info.Register,
			logical.Register,
			lookup.Register,
			mathtrig.Register,
			text.Register,
		} {
			register(registry)
		}
	})
	return registry
}
