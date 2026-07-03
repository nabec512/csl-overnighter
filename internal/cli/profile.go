package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nabec512/csl-overnighter/internal/profile"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage saved permit profiles",
	}

	cmd.AddCommand(newProfileSaveCmd())
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileShowCmd())
	cmd.AddCommand(newProfileDeleteCmd())

	return cmd
}

func newProfileSaveCmd() *cobra.Command {
	var rawFields []string

	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save (or overwrite) a profile with the given fields",
		Long: "Save (or overwrite) a profile with the given fields.\n\n" +
			"Fields are passed as repeated --field key=value flags, e.g.:\n" +
			"  csl-overnighter profile save driveway --field plate=ABC1234 --field address=\"123 Main St\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			fields, err := profile.ParseFieldFlags(rawFields)
			if err != nil {
				return err
			}

			store, err := openStore()
			if err != nil {
				return err
			}

			p := &profile.Profile{Name: name, Fields: fields}
			if err := store.Save(p); err != nil {
				return err
			}

			fmt.Printf("Saved profile %q with %d field(s) to %s\n", name, len(fields), store.Dir)
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&rawFields, "field", nil, "a form field as key=value (repeatable)")

	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}

			names, err := store.List()
			if err != nil {
				return err
			}

			if len(names) == 0 {
				fmt.Println("No profiles saved yet. Use `csl-overnighter profile save <name>` to create one.")
				return nil
			}

			for _, n := range names {
				fmt.Println(n)
			}
			return nil
		},
	}
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a saved profile's fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}

			p, err := store.Load(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name:    %s\n", p.Name)
			fmt.Printf("Created: %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Updated: %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))
			fmt.Println("Fields:")
			for k, v := range p.Fields {
				fmt.Printf("  %s = %s\n", k, v)
			}
			return nil
		},
	}
}

func newProfileDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted profile %q\n", args[0])
			return nil
		},
	}
}
