import { useCallback, useEffect, useState } from "react";
import Alert from "@mui/material/Alert";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import Typography from "@mui/material/Typography";

import { errorMessage, getStatus, resumeAll, toast, type Status } from "./api";
import { ActivityLog } from "./components/ActivityLog";
import { ContainersTable } from "./components/ContainersTable";
import { SensorCard } from "./components/SensorCard";
import { Settings } from "./components/Settings";

const POLL_MS = 2000;

export function App() {
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [resuming, setResuming] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setStatus(await getStatus());
      setError(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }, []);

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, POLL_MS);
    return () => clearInterval(timer);
  }, [refresh]);

  const onResume = async () => {
    setResuming(true);
    try {
      setStatus(await resumeAll());
      toast().success("Containers resumed. They stay running until temperatures recover.");
    } catch (err) {
      toast().error(errorMessage(err));
    } finally {
      setResuming(false);
    }
  };

  if (!status) {
    return (
      <Stack alignItems="center" sx={{ mt: 8 }} spacing={2}>
        {error ? <Alert severity="error">{error}</Alert> : <CircularProgress />}
      </Stack>
    );
  }

  const paused = status.paused ?? [];
  const cfg = status.config;

  return (
    <Stack spacing={3} sx={{ p: 1 }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" flexWrap="wrap" gap={1}>
        <Stack>
          <Typography variant="h2">GPU Governor</Typography>
          <Typography variant="body2" color="text.secondary">
            Pauses containers when the CPU or GPU runs too hot and resumes them once it cools down. Paused
            containers keep their memory and GPU state, so no work is lost.
          </Typography>
        </Stack>
        {!cfg.enabled ? (
          <Chip label="Turned off" />
        ) : status.hot ? (
          <Chip color="error" label={`Cooling down, ${paused.length} paused`} />
        ) : (
          <Chip color="success" label="Watching" />
        )}
      </Stack>

      {error && <Alert severity="warning">Lost contact with the backend: {error}</Alert>}
      {status.error && <Alert severity="error">{status.error}</Alert>}

      <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap>
        <SensorCard
          title="GPU"
          sensor={status.gpu}
          limits={cfg.gpu}
          unavailableHint="No NVIDIA GPU found. GPU containers need Docker Desktop on Windows with WSL 2, or Linux with the NVIDIA Container Toolkit."
        />
        <SensorCard
          title="CPU"
          sensor={status.cpu}
          limits={cfg.cpu}
          unavailableHint="This machine does not expose CPU temperature to Docker. Docker Desktop's VM usually hides it; on Linux run the standalone container instead."
        />
      </Stack>

      {paused.length > 0 && (
        <Alert
          severity="warning"
          action={
            <Button color="inherit" size="small" onClick={onResume} disabled={resuming}>
              Resume now
            </Button>
          }
        >
          {paused.length === 1
            ? `${paused[0].name} is paused (${paused[0].reason}).`
            : `${paused.length} containers are paused (${paused[0].reason}).`}{" "}
          They resume automatically once temperatures drop.
        </Alert>
      )}

      <Paper variant="outlined" sx={{ p: 2 }}>
        <Typography variant="h3" gutterBottom>
          Containers
        </Typography>
        <ContainersTable containers={status.containers ?? []} />
      </Paper>

      <Paper variant="outlined" sx={{ p: 2 }}>
        <Typography variant="h3" gutterBottom>
          Settings
        </Typography>
        <Settings config={cfg} onSaved={(config) => setStatus({ ...status, config })} />
      </Paper>

      <Paper variant="outlined" sx={{ p: 2 }}>
        <Typography variant="h3" gutterBottom>
          Activity
        </Typography>
        <ActivityLog events={status.events ?? []} />
      </Paper>
    </Stack>
  );
}
