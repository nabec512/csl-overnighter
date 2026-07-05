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
	var p profile.Profile

	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save (or overwrite) a profile with the given fields",
		Long: "Save (or overwrite) a profile with the given fields.\n\n" +
			"Example:\n" +
			"  csl-overnighter profile save driveway \\\n" +
			"    --address \"5150 AVENUE MACDONALD, Côte Saint-Luc\" \\\n" +
			"    --first-name Jane --last-name Doe \\\n" +
			"    --phone 5145551234 --email jane@example.com \\\n" +
			"    --plate ABC1234 --make Toyota --model Corolla --color Grey \\\n" +
			"    --country Canada --state Quebec --reason \"No driveway\"\n\n" +
			"Running save again on the same <name> overwrites the existing profile;\n" +
			"unset flags reset that field to empty, so pass the full set of flags\n" +
			"each time (not just the ones you're changing).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Name = args[0]
			if err := p.Validate(); err != nil {
				return err
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			if err := store.Save(&p); err != nil {
				return err
			}

			fmt.Printf("Saved profile %q to %s\n", p.Name, store.Dir)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&p.Address, "address", "", "applicant's address, as it appears in the town's address lookup (required)")
	f.StringVar(&p.Suite, "suite", "", "apartment/suite number (optional)")
	f.StringVar(&p.FirstName, "first-name", "", "applicant first name (required)")
	f.StringVar(&p.LastName, "last-name", "", "applicant last name (required)")
	f.StringVar(&p.Phone, "phone", "", "applicant phone number, any format (required)")
	f.StringVar(&p.Email, "email", "", "applicant email, confirmation is sent here (required)")
	f.StringVar(&p.LicencePlate, "plate", "", "licence plate, no spaces, no letter O (required)")
	f.StringVar(&p.VehicleMake, "make", "", "vehicle make (required)")
	f.StringVar(&p.VehicleModel, "model", "", "vehicle model (required)")
	f.StringVar(&p.VehicleColor, "color", "", "vehicle color (required)")
	f.StringVar(&p.Country, "country", "", "vehicle registration country (required)")
	f.StringVar(&p.State, "state", "", "vehicle registration state/province (required)")
	f.StringVar(&p.Reason, "reason", "", "reason for the permit request (required)")

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

			fmt.Printf("Name:          %s\n", p.Name)
			fmt.Printf("Created:       %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Updated:       %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Address:       %s\n", p.Address)
			fmt.Printf("Suite:         %s\n", p.Suite)
			fmt.Printf("First name:    %s\n", p.FirstName)
			fmt.Printf("Last name:     %s\n", p.LastName)
			fmt.Printf("Phone:         %s\n", p.Phone)
			fmt.Printf("Email:         %s\n", p.Email)
			fmt.Printf("Licence plate: %s\n", p.LicencePlate)
			fmt.Printf("Vehicle make:  %s\n", p.VehicleMake)
			fmt.Printf("Vehicle model: %s\n", p.VehicleModel)
			fmt.Printf("Vehicle color: %s\n", p.VehicleColor)
			fmt.Printf("Country:       %s\n", p.Country)
			fmt.Printf("State:         %s\n", p.State)
			fmt.Printf("Reason:        %s\n", p.Reason)
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
