package blob

import (
	"strings"
	"testing"
)

// poolOf writes strings through a pool and returns the number each got.
func poolOf(p *Pool, parts ...string) []int {
	var nums []int
	for _, s := range parts {
		nums = append(nums, p.intern([]byte(s)))
	}
	return nums
}

// An extension holds only the parts a seeded pool added, and read over its
// base answers every part: the base's under their numbers, the added ones
// after. Extensions chain, and are refused over any other base and alone.
func TestPoolExtension(t *testing.T) {
	base := NewPool(3)
	baseNums := poolOf(base, "Greenwich Mean Time", "British Summer Time", "미 동부 표준시", "")
	baseBytes := base.Bytes()
	baseShared, err := ReadShared(baseBytes, 3)
	if err != nil {
		t.Fatal(err)
	}

	seeded := SeedPool(3, baseShared)
	nums := poolOf(seeded, "British Summer Time", "북미 동부 표준시", "Greenwich Mean Time", "British Time")
	if nums[0] != baseNums[1] || nums[2] != baseNums[0] {
		t.Errorf("parts the base has took %v, want the base's %v", nums, baseNums)
	}
	if nums[1] != 4 || nums[3] != 5 {
		t.Errorf("parts the base lacks took %d and %d, want 4 and 5", nums[1], nums[3])
	}
	ext, err := seeded.Extension()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ext), "Greenwich") {
		t.Error("the extension repeats a part of its base")
	}
	if _, err := ReadShared(ext, 3); err == nil {
		t.Error("an extension read as a pool on its own")
	}
	extended, err := Extend(baseShared, ext, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Greenwich Mean Time", "British Summer Time", "미 동부 표준시", "", "북미 동부 표준시", "British Time"}
	if extended.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", extended.Len(), len(want))
	}
	for i, w := range want {
		if got, ok := extended.part(i); !ok || string(got) != w {
			t.Errorf("part %d = %q %v, want %q", i, got, ok, w)
		}
	}
	if _, ok := extended.part(len(want)); ok {
		t.Error("a part past the extension's end")
	}

	// A further extension, of the extended pool.
	again := SeedPool(3, extended)
	if n := poolOf(again, "Irish Time", "북미 동부 표준시"); n[0] != 6 || n[1] != 4 {
		t.Errorf("the second extension numbered %v, want [6 4]", n)
	}
	ext2, err := again.Extension()
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Extend(extended, ext2, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range append(want, "Irish Time") {
		if got, ok := twice.part(i); !ok || string(got) != w {
			t.Errorf("twice-extended part %d = %q %v, want %q", i, got, ok, w)
		}
	}

	// Over the wrong base, or another version, it is refused.
	other := NewPool(3)
	poolOf(other, "Greenwich Mean Time")
	otherShared, _ := ReadShared(other.Bytes(), 3)
	if _, err := Extend(otherShared, ext, 3); err == nil {
		t.Error("an extension read over a base it was not made over")
	}
	if _, err := Extend(baseShared, ext, 4); err == nil {
		t.Error("an extension read as another version")
	}
	if _, err := NewPool(3).Extension(); err == nil {
		t.Error("an extension of a pool that was not seeded")
	}

	// A table whose parts are in the extension finds them.
	w := NewPooledWriter(3, again)
	w.SharedTable([]string{"Irish Time", "Greenwich Mean Time"}, func(key string, w *Writer) { w.Uint(len(key)) })
	tableExt, _ := again.Extension()
	pool, err := Extend(extended, tableExt, 3)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewPooledReader(w.Bytes(), 3, pool)
	if err != nil {
		t.Fatal(err)
	}
	table := r.SharedTable()
	if rec, ok := table.Find("Irish Time"); !ok || rec.Uint() != len("Irish Time") {
		t.Error("a table key in the extension was not found")
	}
}
