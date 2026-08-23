import {useCurrentFrame} from 'remotion';
import {DesktopStage} from '../components/DesktopStage';
import {FlowPacket} from '../components/FlowPacket';

export const FailureScene: React.FC = () => {
  const frame = useCurrentFrame();
  return <DesktopStage consumer="failure" broker={frame < 35 ? 'healthy17' : 'attention17'} extension="healthy17" failureRevealAt={0}>
    <FlowPacket fromX={330} toX={565} y={366} start={11} end={38} label="authentication_required" tone="attention" />
  </DesktopStage>;
};
