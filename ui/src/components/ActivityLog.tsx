import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import ListItemText from "@mui/material/ListItemText";
import Typography from "@mui/material/Typography";

import type { GovernorEvent } from "../api";

const levelColor = {
  info: "text.primary",
  warn: "warning.main",
  error: "error.main",
} as const;

export function ActivityLog({ events }: { events: GovernorEvent[] }) {
  if (events.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        Nothing yet.
      </Typography>
    );
  }
  return (
    <List dense sx={{ maxHeight: 260, overflow: "auto" }}>
      {[...events].reverse().map((e, i) => (
        <ListItem key={`${e.time}-${i}`} disableGutters>
          <ListItemText
            primary={e.message}
            secondary={new Date(e.time).toLocaleString()}
            primaryTypographyProps={{ color: levelColor[e.level] ?? "text.primary" }}
          />
        </ListItem>
      ))}
    </List>
  );
}
