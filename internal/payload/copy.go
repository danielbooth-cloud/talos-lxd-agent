package payload

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Clean removes every entry in root except the named entries.
func Clean(root string, keep ...string) error {
	kept := make(map[string]struct{}, len(keep))
	for _, name := range keep {
		kept[name] = struct{}{}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if _, ok := kept[entry.Name()]; ok {
			continue
		}

		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

// CopyTree copies regular files, directories, and symbolic links from src to dst.
func CopyTree(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case entry.Type()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}

			if err := os.RemoveAll(target); err != nil {
				return err
			}

			return os.Symlink(linkTarget, target)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported file type %s at %q", info.Mode().Type(), path)
		}
	})
}

func copyFile(src, dst string, mode fs.FileMode) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); retErr == nil && err != nil {
			retErr = err
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Chmod(mode)
}
