import { writable } from 'svelte/store';

export type Step = 'controller' | 'identity' | 'queue' | 'selection' | 'fight';
export type ControlMode = 'one-stick' | 'two-stick';

export type TelemetryData = {
  wifiRssi?: number;
  throttle?: number;
  steering?: number;
  motor1?: number;
  motor2?: number;
  escArmMode?: boolean;
  armRemainingMs?: number;
};

export type FightState = {
  step: Step;
  controllerReady: boolean;
  gamepadIndex: number | null;
  controlMode: ControlMode;
  fightId: number | null;
  opponent: string;
  botId: string;
  bots: Array<{ id: string; name: string; online: boolean; enabled: boolean }>;
  telemetry: TelemetryData;
  pingMs: number;
  timerRemainingSec: number;
  startedAtServer: number;
  durationSec: number;
};

export const fight = writable<FightState>({
  step: 'controller',
  controllerReady: false,
  gamepadIndex: null,
  controlMode: 'one-stick',
  fightId: null,
  opponent: '',
  botId: '',
  bots: [],
  telemetry: {},
  pingMs: 0,
  timerRemainingSec: 0,
  startedAtServer: 0,
  durationSec: 180
});

export function resetFlowAfterFight() {
  fight.update((f) => ({
    ...f,
    step: 'queue',
    fightId: null,
    opponent: '',
    botId: '',
    telemetry: {},
    timerRemainingSec: 0,
    startedAtServer: 0
  }));
}
