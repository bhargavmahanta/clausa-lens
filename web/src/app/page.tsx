import Image from "next/image";

import { CommandCenterShell } from "../components/system";
import { CommandCenter } from "../features/command-center";

export default function HomePage() {
  return (
    <CommandCenterShell>
      <div className="ambient-scene" aria-hidden="true">
        <span className="ambient-light ambient-light--one" />
        <span className="ambient-light ambient-light--two" />
        <span className="ambient-light ambient-light--three" />
        <Image alt="" className="ambient-object ambient-object--ring" height={1199} sizes="(max-width: 720px) 220px, 28vw" src="/figma/ambient-ring.png" width={1312} />
        <Image alt="" className="ambient-object ambient-object--plant" height={1285} loading="eager" sizes="(max-width: 720px) 250px, 28vw" src="/figma/ambient-plant.png" width={1224} />
        <Image alt="" className="ambient-object ambient-object--stone" height={1024} sizes="(max-width: 720px) 1px, 30vw" src="/figma/ambient-stone.png" width={1536} />
        <Image alt="" className="ambient-object ambient-object--monolith" height={1536} sizes="(max-width: 720px) 150px, 17vw" src="/figma/ambient-monolith.png" width={1024} />
        <Image alt="" className="ambient-object ambient-object--orb" height={1254} loading="eager" sizes="(max-width: 720px) 1px, 16vw" src="/figma/ambient-orb.png" width={1254} />
      </div>

      <CommandCenter />
    </CommandCenterShell>
  );
}
