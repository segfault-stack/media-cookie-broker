import {TransitionSeries} from '@remotion/transitions';
import {HealthyScene} from './scenes/HealthyScene';
import {FailureScene} from './scenes/FailureScene';
import {NotificationScene} from './scenes/NotificationScene';
import {HumanActionScene} from './scenes/HumanActionScene';
import {RecoveryScene} from './scenes/RecoveryScene';
import {FreshRevisionScene} from './scenes/FreshRevisionScene';
import {PayoffScene} from './scenes/PayoffScene';

export const MediaCookieBrokerDemo: React.FC = () => (
  <TransitionSeries>
    <TransitionSeries.Sequence durationInFrames={51} name="1 · Healthy session"><HealthyScene /></TransitionSeries.Sequence>
    <TransitionSeries.Sequence durationInFrames={45} name="2 · Consumer reports authentication failure"><FailureScene /></TransitionSeries.Sequence>
    <TransitionSeries.Sequence durationInFrames={45} name="3 · Human gets notified"><NotificationScene /></TransitionSeries.Sequence>
    <TransitionSeries.Sequence durationInFrames={45} name="4 · Explicit human action"><HumanActionScene /></TransitionSeries.Sequence>
    <TransitionSeries.Sequence durationInFrames={75} name="5 · Isolated browser recovery"><RecoveryScene /></TransitionSeries.Sequence>
    <TransitionSeries.Sequence durationInFrames={54} name="6 · Fresh revision"><FreshRevisionScene /></TransitionSeries.Sequence>
    <TransitionSeries.Sequence durationInFrames={75} name="7 · Healthy payoff"><PayoffScene /></TransitionSeries.Sequence>
  </TransitionSeries>
);
