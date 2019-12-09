package fakego

type variant_container_base struct {
	recurflag int32
}

type variant_array struct {
	variant_container_base
	va []*variant
}

type variant_map struct {
	variant_container_base
}
