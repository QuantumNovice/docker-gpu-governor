import { useEffect, useState } from "react";
import Button from "@mui/material/Button";
import FormControlLabel from "@mui/material/FormControlLabel";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

import { errorMessage, saveConfig, toast, type Config, type Limits, type Mode } from "../api";

const modeHelp: Record<Mode, string> = {
  gpu: "Containers started with --gpus. Add the label gpu-governor.enable=true to include others.",
  labeled: "Only containers labeled gpu-governor.enable=true.",
  all: "Every running container except those labeled gpu-governor.exclude=true.",
};

interface Props {
  config: Config;
  onSaved: (cfg: Config) => void;
}

export function Settings({ config, onSaved }: Props) {
  const [draft, setDraft] = useState<Config>(config);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  // Follow the backend until the user starts editing.
  useEffect(() => {
    if (!dirty) setDraft(config);
  }, [config, dirty]);

  const update = (patch: Partial<Config>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setDirty(true);
  };

  const save = async (cfg: Config) => {
    setSaving(true);
    try {
      const saved = await saveConfig(cfg);
      setDraft(saved);
      setDirty(false);
      onSaved(saved);
      toast().success("Settings saved");
    } catch (err) {
      toast().error(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Stack spacing={2}>
      <FormControlLabel
        control={
          <Switch
            checked={draft.enabled}
            disabled={saving}
            onChange={(e) => save({ ...draft, enabled: e.target.checked })}
          />
        }
        label={draft.enabled ? "Governor is on" : "Governor is off (everything it paused is resumed)"}
      />

      <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap>
        <TextField
          select
          label="Containers to pause"
          value={draft.mode}
          onChange={(e) => update({ mode: e.target.value as Mode })}
          helperText={modeHelp[draft.mode]}
          sx={{ minWidth: 260, maxWidth: 420 }}
        >
          <MenuItem value="gpu">GPU containers</MenuItem>
          <MenuItem value="labeled">Labeled containers</MenuItem>
          <MenuItem value="all">All containers</MenuItem>
        </TextField>
        <NumberField
          label="Check every (seconds)"
          value={draft.intervalSeconds}
          onChange={(v) => update({ intervalSeconds: v })}
        />
      </Stack>

      <LimitsEditor title="GPU" limits={draft.gpu} onChange={(gpu) => update({ gpu })} />
      <LimitsEditor title="CPU" limits={draft.cpu} onChange={(cpu) => update({ cpu })} />

      <Stack direction="row" spacing={1}>
        <Button variant="contained" disabled={!dirty || saving} onClick={() => save(draft)}>
          Save
        </Button>
        <Button
          disabled={!dirty || saving}
          onClick={() => {
            setDraft(config);
            setDirty(false);
          }}
        >
          Discard
        </Button>
      </Stack>
    </Stack>
  );
}

function LimitsEditor({ title, limits, onChange }: { title: string; limits: Limits; onChange: (l: Limits) => void }) {
  const set = (patch: Partial<Limits>) => onChange({ ...limits, ...patch });
  return (
    <Stack spacing={1}>
      <FormControlLabel
        control={<Switch checked={limits.enabled} onChange={(e) => set({ enabled: e.target.checked })} />}
        label={<Typography variant="h4">{title} limit</Typography>}
      />
      <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap>
        <NumberField label="Pause at (°C)" value={limits.pauseAtC} disabled={!limits.enabled} onChange={(v) => set({ pauseAtC: v })} />
        <NumberField label="Resume at (°C)" value={limits.resumeAtC} disabled={!limits.enabled} onChange={(v) => set({ resumeAtC: v })} />
        <NumberField
          label="Hot readings before pausing"
          value={limits.tripSamples}
          disabled={!limits.enabled}
          onChange={(v) => set({ tripSamples: v })}
        />
        <NumberField
          label="Cool readings before resuming"
          value={limits.clearSamples}
          disabled={!limits.enabled}
          onChange={(v) => set({ clearSamples: v })}
        />
      </Stack>
    </Stack>
  );
}

function NumberField(props: { label: string; value: number; disabled?: boolean; onChange: (v: number) => void }) {
  return (
    <TextField
      type="number"
      label={props.label}
      value={Number.isFinite(props.value) ? props.value : ""}
      disabled={props.disabled}
      onChange={(e) => props.onChange(e.target.value === "" ? NaN : Number(e.target.value))}
      sx={{ width: 220 }}
    />
  );
}
