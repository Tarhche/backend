import {List, ListItem} from "@mantine/core";

type Props = {
  errors?: (string | undefined | null)[];
};

export function FieldErrors({errors = []}: Props) {
  if (errors.length === 0) return null;
  return (
    <List mt={0}>
      {errors.map((e) => {
        if (Boolean(e) === false) {
          return null;
        }
        return (
          <ListItem fz={"xs"} c={"red"} key={e}>
            {e}
          </ListItem>
        );
      })}
    </List>
  );
}
