package gotindeq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewTindeqClient(t *testing.T) {
	listTests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "init client",
			wantErr: false,
		},
	}

	for _, tt := range listTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTindeqClient()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

}
