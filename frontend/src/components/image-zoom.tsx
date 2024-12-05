import Zoom, {UncontrolledProps} from "react-medium-image-zoom";
import "react-medium-image-zoom/dist/styles.css";
import classes from "./image-zoom.module.css";

type Props = UncontrolledProps;

export function ImageZoom(props: Props) {
  return (
    <Zoom {...props} classDialog={classes.rmiz}>
      {props.children}
    </Zoom>
  );
}
