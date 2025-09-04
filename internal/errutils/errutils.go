package errutils

func LogIfErr(err error) {
	_ = err
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}
