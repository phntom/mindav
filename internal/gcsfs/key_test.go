package gcsfs

import "testing"

func TestToKey(t *testing.T) {
	cases := map[string]string{
		"/":                       "",
		"":                        "",
		"/hello.txt":              "hello.txt",
		"hello.txt":               "hello.txt",
		"/u/a@b.com/my.kdbx":      "u/a@b.com/my.kdbx",
		"/foo/../bar":             "bar",
		"/foo/./bar/":             "foo/bar",
		"//double//slash":         "double/slash",
		"/trailing/":              "trailing",
	}
	for in, want := range cases {
		if got := toKey(in); got != want {
			t.Errorf("toKey(%q) = %q, want %q", in, got, want)
		}
	}
}
