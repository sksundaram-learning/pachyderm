package worker

import (
	"fmt"

	"github.com/pachyderm/pachyderm/src/client"
	"github.com/pachyderm/pachyderm/src/client/pfs"
	"github.com/pachyderm/pachyderm/src/client/pps"

	"golang.org/x/net/context"
)

// DatumFactory is an interface which allows you to iterate through the datums
// for a job.
type DatumFactory interface {
	Len() int
	Datum(i int) []*Input
}

type atomDatumFactory struct {
	inputs []*Input
	index  int
}

func newAtomDatumFactory(ctx context.Context, pfsClient pfs.APIClient, input *pps.AtomInput) (DatumFactory, error) {
	result := &atomDatumFactory{}
	fileInfos, err := pfsClient.GlobFile(ctx, &pfs.GlobFileRequest{
		Commit:  client.NewCommit(input.Repo, input.Commit),
		Pattern: input.Glob,
	})
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfos.FileInfo {
		result.inputs = append(result.inputs, &Input{
			FileInfo: fileInfo,
			Name:     input.Name,
			Lazy:     input.Lazy,
			Branch:   input.Branch,
		})
	}
	return result, nil
}

func (d *atomDatumFactory) Len() int {
	return len(d.inputs)
}

func (d *atomDatumFactory) Datum(i int) []*Input {
	return []*Input{d.inputs[i]}
}

type unionDatumFactory struct {
	inputs []DatumFactory
}

func newUnionDatumFactory(ctx context.Context, pfsClient pfs.APIClient, union []*pps.Input) (DatumFactory, error) {
	result := &unionDatumFactory{}
	for _, input := range union {
		datumFactory, err := NewDatumFactory(ctx, pfsClient, input)
		if err != nil {
			return nil, err
		}
		result.inputs = append(result.inputs, datumFactory)
	}
	return result, nil
}

func (d *unionDatumFactory) Len() int {
	result := 0
	for _, datumFactory := range d.inputs {
		result += datumFactory.Len()
	}
	return result
}

func (d *unionDatumFactory) Datum(i int) []*Input {
	for _, datumFactory := range d.inputs {
		if i < datumFactory.Len() {
			return datumFactory.Datum(i)
		}
		i -= datumFactory.Len()
	}
	panic("index out of bounds")
}

type crossDatumFactory struct {
	inputs []DatumFactory
}

func (d *crossDatumFactory) Len() int {
	if len(d.inputs) == 0 {
		return 0
	}
	result := d.inputs[0].Len()
	for i := 1; i < len(d.inputs); i++ {
		result *= d.inputs[i].Len()
	}
	return result
}

func (d *crossDatumFactory) Datum(i int) []*Input {
	if i >= d.Len() {
		panic("index out of bounds")
	}
	var result []*Input
	for _, datumFactory := range d.inputs {
		result = append(result, datumFactory.Datum(i%datumFactory.Len())...)
		i /= datumFactory.Len()
	}
	return result
}

func newCrossDatumFactory(ctx context.Context, pfsClient pfs.APIClient, cross []*pps.Input) (DatumFactory, error) {
	result := &crossDatumFactory{}
	for _, input := range cross {
		datumFactory, err := NewDatumFactory(ctx, pfsClient, input)
		if err != nil {
			return nil, err
		}
		result.inputs = append(result.inputs, datumFactory)
	}
	return result, nil
}

func newCronDatumFactory(ctx context.Context, pfsClient pfs.APIClient, input *pps.CronInput) (DatumFactory, error) {
	return newAtomDatumFactory(ctx, pfsClient, &pps.AtomInput{
		Name:   input.Name,
		Repo:   input.Repo,
		Branch: "master",
		Commit: input.Commit,
		Glob:   "time",
	})
}

// NewDatumFactory creates a datumFactory for an input.
func NewDatumFactory(ctx context.Context, pfsClient pfs.APIClient, input *pps.Input) (DatumFactory, error) {
	switch {
	case input.Atom != nil:
		return newAtomDatumFactory(ctx, pfsClient, input.Atom)
	case input.Union != nil:
		return newUnionDatumFactory(ctx, pfsClient, input.Union)
	case input.Cross != nil:
		return newCrossDatumFactory(ctx, pfsClient, input.Cross)
	case input.Cron != nil:
		return newCronDatumFactory(ctx, pfsClient, input.Cron)
	}
	return nil, fmt.Errorf("unrecognized input type")
}
