package example

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) GetExample() Response {
	return Response{Status: "example route works"}
}
