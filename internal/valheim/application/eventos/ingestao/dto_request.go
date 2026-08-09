package ingestao

// IngestaoRequestDto é o corpo JSON aceito em `POST /eventos`.
//
// Os dois campos existem porque há dois jeitos de o hook do container chamar:
//
//	# uma linha por chamada (o exemplo do README do valheim-server-docker)
//	{ read l; curl ... -d "{\"log\":\"$l\"}" ... }
//
//	# várias linhas de uma vez (quem acumula antes de mandar)
//	{"logs": ["...", "..."]}
//
// E há um terceiro, que é o recomendado e não passa por aqui: mandar a linha
// CRUA com `Content-Type: text/plain`. Ver o comentário do controller.
type IngestaoRequestDto struct {
	Log  string   `json:"log"`
	Logs []string `json:"logs"`
}

// Linhas junta as duas formas numa lista só.
func (d IngestaoRequestDto) Linhas() []string {
	linhas := make([]string, 0, len(d.Logs)+1)
	if d.Log != "" {
		linhas = append(linhas, d.Log)
	}
	linhas = append(linhas, d.Logs...)
	return linhas
}
