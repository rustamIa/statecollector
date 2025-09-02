package textutil

import (
	"fmt"
	"testing"
)

func TestSplitN(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		numCols  int
		wantCols []string
		wantOk   bool
	}{
		{
			name:     "valid line with 4 columns",
			line:     "US;36;1576;Rond",
			numCols:  4,
			wantCols: []string{"US", "36", "1576", "Rond"},
			wantOk:   true,
		},
		{
			name:     "valid line with 3 columns",
			line:     "RU;86;Rond",
			numCols:  4,
			wantCols: nil,
			wantOk:   false,
		},
		{
			name:     "line with extra column",
			line:     "BL;68;1594;Kildy;10500",
			numCols:  4,
			wantCols: nil,
			wantOk:   false,
		},
		{
			name:     "line with missing column",
			line:     "GB28495Topolo",
			numCols:  4,
			wantCols: nil,
			wantOk:   false,
		},
		{
			name:     "line with another separator",
			line:     "GB 8p8 1892 Topolo 1 f",
			numCols:  4,
			wantCols: nil,
			wantOk:   false,
		},
		{
			name:     "empty line",
			line:     "",
			numCols:  4,
			wantCols: nil,
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCols, gotOk := SplitN(tt.line, ';', tt.numCols)
			if gotOk != tt.wantOk {
				t.Errorf("SplitN() ok = %v, want %v", gotOk, tt.wantOk)
			}
			if fmt.Sprintf("%v", gotCols) != fmt.Sprintf("%v", tt.wantCols) {
				t.Errorf("SplitN() = %v, want %v", gotCols, tt.wantCols)
			}
		})
	}
}
