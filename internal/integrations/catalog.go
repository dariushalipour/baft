package integrations

import (
	"context"
	"errors"
	"sort"
)

const (
	FamilyVSCode    = "vscode"
	FamilyJetBrains = "jetbrains"
)

type IDEInstallation struct {
	ID          string
	Family      string
	DisplayName string
	Version     string
	InstallPath string
	Executable  string
	PluginDir   string
	IsRunning   bool
}

type Installer interface {
	Family() string
	Detect(context.Context) ([]IDEInstallation, error)
	Install(context.Context, IDEInstallation) error
	Verify(context.Context, IDEInstallation) error
}

type Catalog struct {
	cliVersion string
	installers map[string]Installer
}

func NewCatalog(cliVersion string) *Catalog {
	items := []Installer{
		newVSCodeInstaller(cliVersion),
		newJetBrainsInstaller(cliVersion),
	}
	installers := make(map[string]Installer, len(items))
	for _, installer := range items {
		installers[installer.Family()] = installer
	}
	return &Catalog{cliVersion: cliVersion, installers: installers}
}

func (c *Catalog) Detect(ctx context.Context) ([]IDEInstallation, error) {
	families := make([]string, 0, len(c.installers))
	for family := range c.installers {
		families = append(families, family)
	}
	sort.Strings(families)

	var found []IDEInstallation
	var errs []error
	seen := make(map[string]struct{})
	for _, family := range families {
		installations, err := c.installers[family].Detect(ctx)
		if err != nil {
			errs = append(errs, err)
		}
		for _, ide := range installations {
			key := ide.Family + ":" + ide.ID + ":" + ide.Executable + ":" + ide.InstallPath
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			found = append(found, ide)
		}
	}
	return found, errors.Join(errs...)
}

func (c *Catalog) Install(ctx context.Context, ide IDEInstallation) error {
	installer, err := c.installer(ide.Family)
	if err != nil {
		return err
	}
	return installer.Install(ctx, ide)
}

func (c *Catalog) Verify(ctx context.Context, ide IDEInstallation) error {
	installer, err := c.installer(ide.Family)
	if err != nil {
		return err
	}
	return installer.Verify(ctx, ide)
}

func (c *Catalog) VerifyCompatibility(integrationID, pluginVersion string, protocol int) CompatibilityReport {
	return VerifyCompatibility(c.cliVersion, integrationID, pluginVersion, protocol)
}

func (c *Catalog) installer(family string) (Installer, error) {
	installer, ok := c.installers[family]
	if !ok {
		return nil, errors.New("unsupported integration family: " + family)
	}
	return installer, nil
}
