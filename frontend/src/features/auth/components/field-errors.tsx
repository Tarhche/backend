import {List, ListItem} from "@mantine/core";

type Props = {
  errors?: string[];
};

export function FieldErrors({errors = []}: Props) {
  if (errors.length === 0) return null;
  return (
    <List mt={0}>
      {errors.map((e) => {
        return (
          <ListItem fz={"xs"} c={"red"} key={e}>
            {e}
          </ListItem>
        );
      })}
    </List>
  );
}
