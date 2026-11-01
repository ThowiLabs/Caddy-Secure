package sitecli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

func groupID(group string) (int, error) {
	g, err := user.LookupGroup(group)
	if err != nil {
		return 0, fmt.Errorf("no existe el grupo %q: %w (comprueba instalación del usuario caddy)", group, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, err
	}
	return gid, nil
}

func securePath(path string, isDir bool, group string) error {
	gid, err := groupID(group)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("se rechaza enlace simbólico: %s", path)
	}
	if fi.IsDir() != isDir {
		return fmt.Errorf("tipo de recurso inesperado: %s", path)
	}
	var mode os.FileMode = 0640
	if isDir {
		mode = 0750
	}
	if err = os.Chmod(path, mode); err != nil {
		return err
	}
	// Do not silently change an existing file in add: this only runs on newly
	// created files, or when doctor was explicitly invoked with --fix-permissions.
	if os.Geteuid() == 0 {
		if err = os.Chown(path, 0, gid); err != nil {
			return err
		}
	}
	return nil
}

func doctor(root, group string, fix bool, w io.Writer) error {
	if err := ensureRealDirectories(root); err != nil {
		return err
	}
	if root == "/" || !filepath.IsAbs(root) {
		return errors.New("ruta raíz insegura")
	}
	gid, err := groupID(group)
	if err != nil {
		return err
	}
	var issues, checked, changed int
	// First pass checks the entire tree for symlinks or special files before any
	// changes are made. In particular, doctor never follows symlinks.
	err = filepath.WalkDir(root, func(path string, e os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("enlace simbólico encontrado, no se modificó nada: %s", path)
		}
		if !e.IsDir() && !e.Type().IsRegular() {
			return fmt.Errorf("archivo especial no permitido: %s", path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	err = filepath.Walk(root, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		checked++
		desired := os.FileMode(0640)
		if fi.IsDir() {
			desired = 0750
		}
		uid, owner := ownerIDs(fi)
		wrong := (fi.Mode().Perm() != desired) || (owner != -1 && owner != gid) || (uid != -1 && uid != 0)
		if !wrong {
			return nil
		}
		issues++
		fmt.Fprintf(w, "REVISAR %s (actual %04o, recomendado %04o, gid actual %d, gid esperado %d)\n", path, fi.Mode().Perm(), desired, owner, gid)
		if fix {
			if err = securePath(path, fi.IsDir(), group); err != nil {
				return err
			}
			changed++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if issues == 0 {
		fmt.Fprintf(w, "OK: %d elementos con permisos correctos. No se realizaron cambios.\n", checked)
	} else if !fix {
		fmt.Fprintf(w, "Se encontraron %d problemas en %d elementos; no se cambió nada. Usa --fix-permissions para corregirlos.\n", issues, checked)
	} else {
		fmt.Fprintf(w, "Corregidos %d elementos de %d revisados.\n", changed, checked)
	}
	return nil
}
