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

package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a string that we used to normalize version string.
type Version string

// splitVersion takes version string, and encapsulates it in comparable []string.
func splitVersion(version string) []string {
	version = strings.Replace(version, "v", "", -1)
	version = strings.Split(version, "-")[0]
	return strings.Split(version, ".")
}

func parseVersionPart(version Version, index int, part string) (int, error) {
	value, err := strconv.Atoi(part)
	if err != nil {
		return 0, fmt.Errorf("error version format %s at index %d: %w", version, index, err)
	}

	return value, nil
}

// GreaterThan if givenVersion >= oldVersion return true, else return false
func (v Version) GreaterThan(oldVersion Version) (bool, error) {
	givenVersion := splitVersion(string(v))
	ov := splitVersion(string(oldVersion))

	if len(givenVersion) != 3 || len(ov) != 3 {
		return false, fmt.Errorf("error version format %s %s", v, oldVersion)
	}

	gMajor, err := parseVersionPart(v, 0, givenVersion[0])
	if err != nil {
		return false, err
	}
	oMajor, err := parseVersionPart(oldVersion, 0, ov[0])
	if err != nil {
		return false, err
	}
	if gMajor > oMajor {
		return true, nil
	} else if gMajor < oMajor {
		return false, nil
	}

	gMinor, err := parseVersionPart(v, 1, givenVersion[1])
	if err != nil {
		return false, err
	}
	oMinor, err := parseVersionPart(oldVersion, 1, ov[1])
	if err != nil {
		return false, err
	}
	if gMinor > oMinor {
		return true, nil
	} else if gMinor < oMinor {
		return false, nil
	}

	gPatch, err := parseVersionPart(v, 2, givenVersion[2])
	if err != nil {
		return false, err
	}
	oPatch, err := parseVersionPart(oldVersion, 2, ov[2])
	if err != nil {
		return false, err
	}
	if gPatch > oPatch {
		return true, nil
	}

	return gPatch == oPatch, nil
}
