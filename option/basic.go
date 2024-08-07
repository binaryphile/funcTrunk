package option

type Basic[T any] struct {
	ok bool
	t  T
}

func BasicOf[T any](t T) Basic[T] {
	return Basic[T]{
		ok: true,
		t:  t,
	}
}

func (o Basic[T]) Get() (T, bool) {
	return o.t, o.ok
}

func (o Basic[T]) IsOk() bool {
	return o.ok
}

func (o Basic[T]) MustGet() T {
	if o.ok {
		return o.t
	}

	panic("option: not ok")
}

func (o Basic[T]) Or(t T) T {
	if o.ok {
		return o.t
	}

	return t
}

func (o Basic[T]) OrZero() (_ T) {
	if o.ok {
		return o.t
	}

	return
}
