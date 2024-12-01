import Zoom, {UncontrolledProps} from "react-medium-image-zoom";
import classes from "./image-zoom.module.css";

type Props = UncontrolledProps;

export function ImageZoom(props: Props) {
  return (
    <Zoom {...props} classDialog={classes.rmiz}>
      {props.children}
    </Zoom>
  );
}
