import Chip from "@mui/material/Chip";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Typography from "@mui/material/Typography";

import type { ContainerStatus } from "../api";

export function ContainersTable({ containers }: { containers: ContainerStatus[] }) {
  if (containers.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No running containers.
      </Typography>
    );
  }

  const sorted = [...containers].sort(
    (a, b) => Number(b.target) - Number(a.target) || a.name.localeCompare(b.name),
  );

  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell>Container</TableCell>
          <TableCell>Image</TableCell>
          <TableCell>GPU</TableCell>
          <TableCell>Governed</TableCell>
          <TableCell>State</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {sorted.map((c) => (
          <TableRow key={c.id}>
            <TableCell>{c.name}</TableCell>
            <TableCell sx={{ maxWidth: 240, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {c.image}
            </TableCell>
            <TableCell>{c.usesGPU ? "Yes" : "No"}</TableCell>
            <TableCell>{c.target ? "Yes" : "No"}</TableCell>
            <TableCell>
              {c.pausedByGovernor ? (
                <Chip size="small" color="warning" label="Paused for cooling" />
              ) : c.state === "paused" ? (
                <Chip size="small" label="Paused" />
              ) : (
                <Chip size="small" color="success" variant="outlined" label="Running" />
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
