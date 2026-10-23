import Card from "@mui/material/Card";
import CardContent from "@mui/material/CardContent";
import Chip from "@mui/material/Chip";
import LinearProgress from "@mui/material/LinearProgress";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";

import type { Limits, SensorStatus } from "../api";

interface Props {
  title: string;
  sensor: SensorStatus;
  limits: Limits;
  unavailableHint: string;
}

export function SensorCard({ title, sensor, limits, unavailableHint }: Props) {
  const readings = sensor.readings ?? [];
  const percent = Math.min(100, (sensor.maxC / limits.pauseAtC) * 100);
  const color = sensor.tripped ? "error" : sensor.maxC >= limits.resumeAtC ? "warning" : "success";

  return (
    <Card variant="outlined" sx={{ flex: 1, minWidth: 260 }}>
      <CardContent>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="h3">{title}</Typography>
          {!limits.enabled ? (
            <Chip size="small" label="Limit off" />
          ) : sensor.tripped ? (
            <Chip size="small" color="error" label="Over limit" />
          ) : sensor.available ? (
            <Chip size="small" color="success" label="Normal" />
          ) : (
            <Chip size="small" label="No sensor" />
          )}
        </Stack>

        {sensor.available ? (
          <>
            <Typography variant="h1" sx={{ mt: 1 }}>
              {sensor.maxC.toFixed(0)}°C
            </Typography>
            <LinearProgress variant="determinate" value={percent} color={color} sx={{ my: 1, height: 6, borderRadius: 3 }} />
            <Typography variant="body2" color="text.secondary">
              Pauses at {limits.pauseAtC}°C, resumes at {limits.resumeAtC}°C
            </Typography>
            {readings.length > 1 && (
              <Stack sx={{ mt: 1 }}>
                {readings.map((r) => (
                  <Stack key={r.name} direction="row" justifyContent="space-between">
                    <Typography variant="caption" color="text.secondary" noWrap sx={{ mr: 1 }}>
                      {r.name}
                    </Typography>
                    <Typography variant="caption">{r.tempC.toFixed(0)}°C</Typography>
                  </Stack>
                ))}
              </Stack>
            )}
            {readings.length === 1 && (
              <Typography variant="caption" color="text.secondary">
                {readings[0].name}
              </Typography>
            )}
          </>
        ) : (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            {sensor.error || unavailableHint}
          </Typography>
        )}
      </CardContent>
    </Card>
  );
}
