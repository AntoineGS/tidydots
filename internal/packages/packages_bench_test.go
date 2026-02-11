package packages

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

// benchRenderer implements config.PathRenderer for benchmarking.
type benchRenderer struct{}

func (r *benchRenderer) RenderString(_, _ string) (string, error) {
	return "true", nil
}

func BenchmarkFilterPackages(b *testing.B) {
	packages := make([]Package, 100)
	for i := range packages {
		packages[i] = Package{
			Name: "test-package",
			Managers: map[PackageManager]ManagerValue{
				Pacman: {PackageName: "test"},
			},
		}
	}

	renderer := &benchRenderer{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = FilterPackages(packages, renderer)
	}
}

func BenchmarkFromApplications(b *testing.B) {
	apps := make([]config.Application, 100)
	for i := range apps {
		apps[i] = config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "test"},
				},
			},
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = FromApplications(apps)
	}
}
