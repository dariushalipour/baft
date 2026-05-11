package typescript_test

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/port"
)

func BenchmarkParseImports(b *testing.B) {
	ts := typescript.Language{}
	content := `import { foo } from "./foo";
import { bar } from "./bar";
import { baz } from "@app/baz";
import qux = require("./qux");
const x = import("./dynamic");
export { foo, bar };
`
	fs := memfs.New()
	fs.WriteFile("/test.ts", []byte(content), 0o644)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ts.ParseImports(fs, "/test.ts")
	}
}

func BenchmarkResolveInternalTarget_Relative(b *testing.B) {
	ts := typescript.Language{}
	capsule := port.Capsule{CapsuleID: "my-app", Dir: "/"}
	fs := memfs.New()
	imports := []string{"./utils/helper", "../lib/foo", "./src/bar.ts", "../../pkg/x/y.ts"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ts.ResolveInternalTarget(fs, port.ImportSpec{Path: imports[i%len(imports)]}, capsule, "src/app.ts")
	}
}

func BenchmarkResolveInternalTarget_Absolute(b *testing.B) {
	ts := typescript.Language{}
	capsule := port.Capsule{CapsuleID: "my-app", Dir: "/"}
	fs := memfs.New()
	fs.WriteFile("/tsconfig.json", []byte(`{"compilerOptions":{"paths":{"@app/*":["src/*"]}}}`), 0o644)
	imports := []string{"@app/utils", "@app/lib/foo", "@app/src/bar"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ts.ResolveInternalTarget(fs, port.ImportSpec{Path: imports[i%len(imports)]}, capsule, "src/app.ts")
	}
}
