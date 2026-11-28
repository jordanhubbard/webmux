//go:build !linux && !darwin

package credentialhub

func checkPrivateDir(string) error            { return ErrUnavailable }
func makePrivateDir(string) error             { return ErrUnavailable }
func readPrivate(string, int) ([]byte, error) { return nil, ErrUnavailable }
func createPrivate(string, []byte) error      { return ErrUnavailable }

func syncPrivateDir(string) error { return ErrUnavailable }
