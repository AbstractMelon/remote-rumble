<script lang="ts">
  export let telemetry: {
    wifiRssi?: number;
    throttle?: number;
    steering?: number;
    motor1?: number;
    motor2?: number;
    escArmMode?: boolean;
    armRemainingMs?: number;
  } = {};

  export let pingMs = 0;

  const pct = (v: number | undefined) => Math.round(Math.abs(v ?? 0) * 100);
  const rssiPct = (v: number | undefined) => {
    const r = v ?? -100;
    return Math.max(0, Math.min(100, Math.round(((r + 100) / 70) * 100)));
  };
</script>

<div class="panel telemetry">
  <h3>Telemetry</h3>

  <div class="row">
    <span>WiFi RSSI ({telemetry.wifiRssi ?? '-'} dBm)</span>
    <div class="meter"><div class="fill" style={`width:${rssiPct(telemetry.wifiRssi)}%`}></div></div>
  </div>
  <div class="row">
    <span>Throttle {pct(telemetry.throttle)}%</span>
    <div class="meter"><div class="fill" style={`width:${pct(telemetry.throttle)}%`}></div></div>
  </div>
  <div class="row">
    <span>Steering {pct(telemetry.steering)}%</span>
    <div class="meter"><div class="fill" style={`width:${pct(telemetry.steering)}%`}></div></div>
  </div>
  <div class="row">
    <span>Motor 1 {pct(telemetry.motor1)}%</span>
    <div class="meter"><div class="fill" style={`width:${pct(telemetry.motor1)}%`}></div></div>
  </div>
  <div class="row">
    <span>Motor 2 {pct(telemetry.motor2)}%</span>
    <div class="meter"><div class="fill" style={`width:${pct(telemetry.motor2)}%`}></div></div>
  </div>

  <div class="chips">
    <span class={telemetry.escArmMode ? 'chip warn' : 'chip ok'}>
      {telemetry.escArmMode ? `ARMING ${telemetry.armRemainingMs ?? 0}ms` : 'DRIVE'}
    </span>
    <span class={pingMs < 80 ? 'chip ok' : pingMs < 200 ? 'chip warn' : 'chip err'}>
      Ping {pingMs || '-'}ms
    </span>
  </div>
</div>

<style>
  .telemetry {
    padding: 1rem;
  }
  h3 {
    margin-top: 0;
  }
  .row {
    margin: 0.45rem 0;
  }
  .row span {
    display: block;
    margin-bottom: 0.2rem;
    font-family: 'IBM Plex Mono', ui-monospace, monospace;
    font-size: 0.84rem;
  }
  .meter {
    height: 10px;
    background: #0d2426;
    border-radius: 999px;
    overflow: hidden;
  }
  .fill {
    height: 100%;
    background: linear-gradient(90deg, #ff9557, #f8ba3d);
  }
  .chips {
    margin-top: 0.8rem;
    display: flex;
    gap: 0.5rem;
  }
  .chip {
    border-radius: 999px;
    padding: 0.2rem 0.62rem;
    font-size: 0.78rem;
  }
  .chip.ok {
    background: rgba(36, 214, 139, 0.15);
    color: #ccffe6;
  }
  .chip.warn {
    background: rgba(248, 186, 61, 0.18);
    color: #ffe6b5;
  }
  .chip.err {
    background: rgba(244, 110, 81, 0.16);
    color: #ffd5cc;
  }
</style>
