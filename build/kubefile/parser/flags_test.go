// Copyright © 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseListFlag(t *testing.T) {
	testDatas := []struct {
		input          string
		expectedKey    string
		expectedValues []string
	}{
		{
			input:          "abc=[a,b,c]",
			expectedKey:    "abc",
			expectedValues: []string{"a", "b", "c"},
		},
		{
			input:          `abc="[a, b,c]"`,
			expectedKey:    "abc",
			expectedValues: []string{"a", "b", "c"},
		},
		{
			input:          `abc=a,b,c`,
			expectedKey:    "abc",
			expectedValues: []string{"a", "b", "c"},
		},
	}

	for _, d := range testDatas {
		fg, err := parseListFlag(d.input)
		if err != nil {
			t.Error(err)
		}

		assert.Equal(t, d.expectedKey, fg.flag)
		assert.Equal(t, d.expectedValues, fg.items)
	}
}
