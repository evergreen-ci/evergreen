package internal

import (
	context "context"

	"github.com/pkg/errors"
	codes "google.golang.org/grpc/codes"
)

func (s *jasperService) ScriptingHarnessSetup(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(id.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", id.Id))
	}

	if err = sh.Setup(ctx); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "setting up scripting harness"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessCleanup(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(id.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", id.Id))
	}

	if err = sh.Cleanup(ctx); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "cleaning up scripting harness"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessRun(ctx context.Context, args *ScriptingHarnessRunArgs) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	if err = sh.Run(ctx, args.Args); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "running scripting command"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessRunScript(ctx context.Context, args *ScriptingHarnessRunScriptArgs) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	err = sh.RunScript(ctx, args.Script)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "running script"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessBuild(ctx context.Context, args *ScriptingHarnessBuildArgs) (*ScriptingHarnessBuildResponse, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	path, err := sh.Build(ctx, args.Directory, args.Args)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "running build"))
	}

	return &ScriptingHarnessBuildResponse{
		Path: path,
		Outcome: &OperationOutcome{
			Success: true,
		}}, nil
}

func (s *jasperService) ScriptingHarnessTest(ctx context.Context, args *ScriptingHarnessTestArgs) (*ScriptingHarnessTestResponse, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	exportedArgs, err := args.Export()
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "exporting arguments"))
	}

	var testErr error
	res, err := sh.Test(ctx, args.Directory, exportedArgs...)
	if err != nil {
		testErr = err
	}
	convertedRes, err := ConvertScriptingTestResults(res)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "converting test results"))
	}

	outcome := &OperationOutcome{Success: testErr == nil}
	if testErr != nil {
		outcome.Text = testErr.Error()
	}
	return &ScriptingHarnessTestResponse{
		Outcome: outcome,
		Results: convertedRes,
	}, nil
}
