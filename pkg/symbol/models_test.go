package symbol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsVendor(t *testing.T) {
	root := "/app"
	if os.PathSeparator == '\\' {
		root = "C:\\app"
	}

	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{
			name:     "Project file",
			filePath: filepath.Join(root, "src/Service/MyService.php"),
			expected: false,
		},
		{
			name:     "Vendor file (relative style)",
			filePath: filepath.Join(root, "vendor/symfony/http-kernel/Kernel.php"),
			expected: true,
		},
		{
			name:     "Vendor file (absolute style outside root)",
			filePath: "/tmp/vendor/package/file.php",
			expected: true,
		},
		{
			name:     "File with vendor in name but not in path",
			filePath: filepath.Join(root, "src/Provider/VendorProvider.php"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := AuditStatus{FilePath: tt.filePath}
			if got := status.IsVendor(root); got != tt.expected {
				t.Errorf("AuditStatus.IsVendor() = %v, want %v (path: %s)", got, tt.expected, tt.filePath)
			}
		})
	}
}

func TestSymfonyService_IsResettable(t *testing.T) {
	tests := []struct {
		name     string
		service  SymfonyService
		expected bool
	}{
		{
			name: "Resettable is true directly",
			service: SymfonyService{
				Resettable: true,
			},
			expected: true,
		},
		{
			name: "Resettable is false and Tags is nil",
			service: SymfonyService{
				Resettable: false,
				Tags:       nil,
			},
			expected: false,
		},
		{
			name: "Resettable is false, Tags is slice of maps containing kernel.reset",
			service: SymfonyService{
				Resettable: false,
				Tags: []any{
					map[string]any{"name": "something_else"},
					map[string]any{"name": "kernel.reset"},
				},
			},
			expected: true,
		},
		{
			name: "Resettable is false, Tags is slice of maps but no kernel.reset",
			service: SymfonyService{
				Resettable: false,
				Tags: []any{
					map[string]any{"name": "something_else"},
				},
			},
			expected: false,
		},
		{
			name: "Resettable is false, Tags is slice but not of maps",
			service: SymfonyService{
				Resettable: false,
				Tags: []any{
					"not_a_map",
				},
			},
			expected: false,
		},
		{
			name: "Resettable is false, Tags is map containing kernel.reset",
			service: SymfonyService{
				Resettable: false,
				Tags: map[string]any{
					"kernel.reset": map[string]any{},
				},
			},
			expected: true,
		},
		{
			name: "Resettable is false, Tags is map without kernel.reset",
			service: SymfonyService{
				Resettable: false,
				Tags: map[string]any{
					"other.tag": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "Resettable is false, Tags is of unsupported type",
			service: SymfonyService{
				Resettable: false,
				Tags:       "unsupported_string",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.service.IsResettable(); got != tt.expected {
				t.Errorf("SymfonyService.IsResettable() = %v, want %v", got, tt.expected)
			}
		})
	}
}
