import {DesktopStage} from '../components/DesktopStage';
import {FlowPacket} from '../components/FlowPacket';
import {RevisionBadge} from '../components/RevisionBadge';
import {clamp} from '../theme';
import {interpolate, useCurrentFrame} from 'remotion';

export const FreshRevisionScene: React.FC = () => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', inset: 0, opacity: interpolate(frame, [0, 8], [0, 1], clamp)}}>
    <DesktopStage consumer="healthy18" broker="healthy18" extension="healthy18">
      <RevisionBadge />
      <FlowPacket fromX={595} toX={280} y={370} start={17} end={49} label="cookies.txt · atomic sync" tone="healthy" />
    </DesktopStage>
  </div>;
};
