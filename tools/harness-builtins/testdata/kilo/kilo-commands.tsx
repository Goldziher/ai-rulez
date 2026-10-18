// Excerpt of https://github.com/Kilo-Org/kilocode/blob/v7.8.14/packages/opencode/src/kilocode/kilo-commands.tsx (lines 50-56, 104-107, 143-153, 166-169, 211-215)
        name: "remote.toggle",
        title: "Toggle remote",
        desc: "Enable or disable remote session relay",
        category: "Kilo",
        slashName: "remote",
        enabled: isKiloConnected(),
        hidden: !isKiloConnected(),
// ...
        category: "Kilo",
        slashName: "profile",
        slashAliases: ["me", "whoami"],
        enabled: isKiloConnected(),
// ...
      ...(indexing()
        ? [
            {
              name: "kilo.indexing",
              title: "Indexing",
              desc: "Configure codebase indexing",
              category: "Kilo",
              slashName: "indexing",
              slashAliases: ["index", "embedding"],
              run: () => {
                dialog.replace(() => <DialogIndexing useSDK={useSDK} />)
// ...
        desc: "Blur PII (balance, email, etc.) and confirm before showing profile",
        category: "Kilo",
        slashName: "privacy",
        run: async () => {
// ...
        category: "Kilo",
        slashName: "teams",
        slashAliases: ["team", "org", "orgs"],
        enabled: isKiloConnected(),
        hidden: !isKiloConnected(),
