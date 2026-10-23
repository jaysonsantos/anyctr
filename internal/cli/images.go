package cli

import (
	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/spec"
)

func newImagesCmd(a *app) *cobra.Command {
	s := &spec.Images{}
	cmd := &cobra.Command{
		Use:     "images [OPTIONS] [REPOSITORY[:TAG]]",
		Short:   "List images",
		Aliases: []string{"image"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				s.Reference = args[0]
			}
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&s.All, "all", "a", false, "Show all images (default hides intermediate images)")
	f.BoolVar(&s.Digests, "digests", false, "Show digests")
	f.StringArrayVarP(&s.Filter, "filter", "f", nil, "Filter output based on conditions provided")
	f.StringVar(&s.Format, "format", "", "Format output using a custom template or json")
	f.BoolVar(&s.NoTrunc, "no-trunc", false, "Don't truncate output")
	f.BoolVarP(&s.Quiet, "quiet", "q", false, "Only show image IDs")
	return flagErrors(cmd)
}

func newBuildCmd(a *app) *cobra.Command {
	s := &spec.Build{}
	cmd := &cobra.Command{
		Use:   "build [OPTIONS] PATH",
		Short: "Build an image from a Dockerfile",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s.Context = "."
			if len(args) == 1 {
				s.Context = args[0]
			}
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&s.AddHost, "add-host", nil, "Add a custom host-to-IP mapping (host:ip)")
	f.StringArrayVar(&s.BuildArg, "build-arg", nil, "Set build-time variables")
	f.StringArrayVar(&s.CacheFrom, "cache-from", nil, "External cache sources")
	f.StringArrayVar(&s.CacheTo, "cache-to", nil, "Cache export destinations")
	f.StringVarP(&s.File, "file", "f", "", "Name of the Dockerfile (default: PATH/Dockerfile)")
	f.StringArrayVar(&s.Label, "label", nil, "Set metadata for an image")
	f.StringVar(&s.Network, "network", "", "Set the networking mode for the RUN instructions")
	f.BoolVar(&s.NoCache, "no-cache", false, "Do not use cache when building the image")
	f.StringVarP(&s.Output, "output", "o", "", "Output destination")
	f.StringVar(&s.Platform, "platform", "", "Set platform if server is multi-platform capable")
	f.StringVar(&s.Progress, "progress", "", `Set type of progress output ("auto", "plain", "tty")`)
	f.BoolVar(&s.Pull, "pull", false, "Always attempt to pull a newer version of the image")
	f.BoolVarP(&s.Quiet, "quiet", "q", false, "Suppress the build output")
	f.StringArrayVar(&s.Secret, "secret", nil, "Secret to expose to the build")
	f.StringArrayVar(&s.SSH, "ssh", nil, "SSH agent socket or keys to expose to the build")
	f.StringArrayVarP(&s.Tag, "tag", "t", nil, "Name and optionally a tag (format: name:tag)")
	f.StringVar(&s.Target, "target", "", "Set the target build stage to build")
	return flagErrors(cmd)
}

func newPullCmd(a *app) *cobra.Command {
	s := &spec.Pull{}
	cmd := &cobra.Command{
		Use:   "pull [OPTIONS] NAME[:TAG|@DIGEST]",
		Short: "Download an image from a registry",
		Args:  requireArgs("pull", 1, "pull [OPTIONS] NAME[:TAG|@DIGEST]"),
		RunE: func(_ *cobra.Command, args []string) error {
			s.Reference = args[0]
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&s.AllTags, "all-tags", "a", false, "Download all tagged images in the repository")
	f.BoolVar(&s.DisableContentTrust, "disable-content-trust", false, "Skip image verification")
	f.StringVar(&s.Platform, "platform", "", "Set platform if server is multi-platform capable")
	f.BoolVarP(&s.Quiet, "quiet", "q", false, "Suppress verbose output")
	return flagErrors(cmd)
}

func newPushCmd(a *app) *cobra.Command {
	s := &spec.Push{}
	cmd := &cobra.Command{
		Use:   "push [OPTIONS] NAME[:TAG]",
		Short: "Upload an image to a registry",
		Args:  requireArgs("push", 1, "push [OPTIONS] NAME[:TAG]"),
		RunE: func(_ *cobra.Command, args []string) error {
			s.Reference = args[0]
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&s.AllTags, "all-tags", "a", false, "Push all tags of an image to the repository")
	f.StringVar(&s.Platform, "platform", "", "Push a platform-specific manifest")
	f.BoolVarP(&s.Quiet, "quiet", "q", false, "Suppress verbose output")
	return flagErrors(cmd)
}

func newRmiCmd(a *app) *cobra.Command {
	s := &spec.Rmi{}
	cmd := &cobra.Command{
		Use:   "rmi [OPTIONS] IMAGE [IMAGE...]",
		Short: "Remove one or more images",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !s.All {
				return requireArgs("rmi", 1, "rmi [OPTIONS] IMAGE [IMAGE...]")(cmd, args)
			}
			s.Images = args
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&s.All, "all", "a", false, "Remove all images (not a Docker flag)")
	f.BoolVarP(&s.Force, "force", "f", false, "Force removal of the image")
	f.BoolVar(&s.NoPrune, "no-prune", false, "Do not delete untagged parents")
	return flagErrors(cmd)
}

func newTagCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]",
		Short: "Create a tag that refers to a source image",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.gateway(&spec.Tag{Source: args[0], Target: args[1]})
		},
	}
	return flagErrors(cmd)
}
