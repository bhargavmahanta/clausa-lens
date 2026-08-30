import { AmbientScene, CommandCenterShell } from "../components/system";
import { CommandCenter } from "../features/command-center";

export default function HomePage() {
  return (
    <CommandCenterShell>
      <AmbientScene />

      <CommandCenter />
    </CommandCenterShell>
  );
}
