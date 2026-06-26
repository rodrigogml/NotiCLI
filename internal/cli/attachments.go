package cli

import "strings"

type attachmentFlags []string

func (a *attachmentFlags) String() string {
	if a == nil {
		return ""
	}
	return strings.Join(*a, ",")
}

func (a *attachmentFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}
