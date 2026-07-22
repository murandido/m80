// Interaction state machine for the grid: sel/editing/sticky per the spec.
// `ac` (autocomplete dropdown state) deliberately lives as local state inside
// each autocomplete-capable cell instead of here - only one cell is ever
// editing at a time, so lifting it bought nothing but extra plumbing.
export interface InteractionState {
  sel: { r: number; c: number }
  editing: boolean
  sticky: boolean
  // Set when edit mode was entered by typing a character outside edit mode
  // (replace-on-type), or by pasting (Ctrl+V), in which case it carries the whole
  // pasted string. The newly-mounted cell consumes it once, then it's cleared via
  // CLEAR_PENDING_CHAR.
  pendingChar: string | null
}

export type InteractionAction =
  | { type: "CLICK_CELL"; r: number; c: number; editable: boolean }
  | { type: "ENTER_EDIT" }
  | { type: "TYPE_REPLACE"; char: string }
  | { type: "ESC" }
  | { type: "MOVE"; r: number; c: number; alsoEdit: boolean }
  | { type: "SELECT_ONLY"; r: number; c: number }
  | { type: "EXIT_EDIT_ONLY" }
  | { type: "CLEAR_PENDING_CHAR" }

export const initialInteractionState: InteractionState = {
  sel: { r: 0, c: 0 },
  editing: false,
  sticky: false,
  pendingChar: null,
}

export function interactionReducer(
  state: InteractionState,
  action: InteractionAction,
): InteractionState {
  switch (action.type) {
    case "CLICK_CELL":
      return {
        sel: { r: action.r, c: action.c },
        editing: action.editable,
        sticky: action.editable,
        pendingChar: null,
      }
    case "ENTER_EDIT":
      return { ...state, editing: true, sticky: true, pendingChar: null }
    case "TYPE_REPLACE":
      return { ...state, editing: true, sticky: true, pendingChar: action.char }
    case "ESC":
      // Autocomplete-open first-stage Esc is handled inside the cell itself
      // (it never dispatches ESC in that case) - reaching here always means
      // stage two: exit edit mode entirely and drop sticky.
      return { ...state, editing: false, sticky: false, pendingChar: null }
    case "MOVE":
      return {
        sel: { r: action.r, c: action.c },
        editing: action.alsoEdit,
        sticky: state.sticky,
        pendingChar: null,
      }
    case "SELECT_ONLY":
      return {
        sel: { r: action.r, c: action.c },
        editing: false,
        sticky: state.sticky,
        pendingChar: null,
      }
    case "EXIT_EDIT_ONLY":
      // Blur for reasons other than Esc: saves and exits edit mode, but -
      // unlike Esc - leaves sticky untouched (only Esc turns it off).
      return { ...state, editing: false, pendingChar: null }
    case "CLEAR_PENDING_CHAR":
      return { ...state, pendingChar: null }
    default:
      return state
  }
}
