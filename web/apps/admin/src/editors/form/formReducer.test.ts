import { describe, expect, it } from "vitest";
import { formEditorReducer as reduce, initFormEditor, newField, newFormDefinition, normalizeForm } from "./formReducer";
import { sampleForm } from "../test/samples";

const init = () => initFormEditor(sampleForm());
const keys = (s: ReturnType<typeof init>) => s.def.fields.map((f) => f.key);

describe("initFormEditor", () => {
  it("starts clean with form settings selected", () => {
    const s = init();
    expect(s).toEqual({ def: sampleForm(), selected: null, dirty: false });
  });

  it("does not share structure with its input", () => {
    const def = sampleForm();
    const s = initFormEditor(def);
    s.def.fields[0].label = "changed";
    expect(def.fields[0].label).toBe("Full name");
  });
});

describe("meta and settings", () => {
  it("sets slug and title even when empty", () => {
    const s = reduce(init(), { type: "setMeta", patch: { slug: "", title: "" } });
    expect(s.def.slug).toBe("");
    expect(s.def.title).toBe("");
    expect(s.dirty).toBe(true);
  });

  it("removes optional meta when cleared", () => {
    let s = reduce(init(), { type: "setMeta", patch: { description: "Hello", workflow: "hiring" } });
    expect(s.def).toMatchObject({ description: "Hello", workflow: "hiring" });
    s = reduce(s, { type: "setMeta", patch: { description: "", workflow: "" } });
    expect("description" in s.def).toBe(false);
    expect("workflow" in s.def).toBe(false);
  });

  it("updates settings and keeps public=false", () => {
    const s = reduce(init(), { type: "setSettings", patch: { public: false, submitLabel: "Send" } });
    expect(s.def.settings).toEqual({ public: false, submitLabel: "Send" });
  });
});

describe("fields", () => {
  it("adds a field with a unique key and selects it", () => {
    const s = reduce(init(), { type: "addField", fieldType: "text" });
    expect(s.def.fields[4]).toEqual({ key: "field1", type: "text", label: "Untitled question" });
    expect(s.selected).toBe(4);
    expect(s.dirty).toBe(true);
  });

  it("adds dropdowns with two starter options", () => {
    const s = reduce(init(), { type: "addField", fieldType: "multiselect" });
    expect(s.def.fields[4].options).toEqual([
      { value: "option1", label: "Option 1" },
      { value: "option2", label: "Option 2" },
    ]);
  });

  it("updates a field and drops cleared optional properties", () => {
    let s = reduce(init(), { type: "updateField", index: 0, patch: { help: "As on your passport" } });
    expect(s.def.fields[0].help).toBe("As on your passport");
    s = reduce(s, { type: "updateField", index: 0, patch: { help: "", required: false } });
    expect("help" in s.def.fields[0]).toBe(false);
    expect("required" in s.def.fields[0]).toBe(false);
  });

  it("ignores out-of-range indexes", () => {
    const s0 = init();
    expect(reduce(s0, { type: "updateField", index: 9, patch: { label: "x" } })).toBe(s0);
    expect(reduce(s0, { type: "removeField", index: -1 })).toBe(s0);
    expect(reduce(s0, { type: "renameField", index: 9, key: "x" })).toBe(s0);
  });

  it("changing to a dropdown adds options and drops validation that no longer applies", () => {
    const s = reduce(init(), { type: "setFieldType", index: 0, fieldType: "select" });
    expect(s.def.fields[0].type).toBe("select");
    expect(s.def.fields[0].options).toHaveLength(2);
    expect(s.def.fields[0].validation).toBeUndefined();
  });

  it("changing away from a dropdown removes options; string validation survives string types", () => {
    expect("options" in reduce(init(), { type: "setFieldType", index: 2, fieldType: "text" }).def.fields[2]).toBe(false);
    expect(reduce(init(), { type: "setFieldType", index: 0, fieldType: "textarea" }).def.fields[0].validation).toEqual({ minLength: 2 });
  });

  it("renaming a key updates conditions that reference it", () => {
    const s = reduce(init(), { type: "renameField", index: 2, key: "position" });
    expect(s.def.fields[2].key).toBe("position");
    expect(s.def.fields[3].showIf).toEqual({ field: "position", equals: "designer" });
  });

  it("renaming one of two duplicate keys leaves conditions alone", () => {
    let s = reduce(init(), { type: "renameField", index: 1, key: "role" });
    s = reduce(s, { type: "renameField", index: 1, key: "email" });
    expect(s.def.fields[3].showIf?.field).toBe("role");
  });

  it("removing a field drops conditions that depended on it", () => {
    const s = reduce(init(), { type: "removeField", index: 2 });
    expect(keys(s)).toEqual(["name", "email", "portfolio"]);
    expect(s.def.fields[2].showIf).toBeUndefined();
  });

  it("removing keeps the selection sensible", () => {
    let s = reduce(init(), { type: "select", index: 3 });
    s = reduce(s, { type: "removeField", index: 3 });
    expect(s.selected).toBe(2);
    s = reduce(s, { type: "removeField", index: 0 });
    expect(s.selected).toBe(1);
    const single = initFormEditor({ ...sampleForm(), fields: [sampleForm().fields[0]] });
    expect(reduce(reduce(single, { type: "select", index: 0 }), { type: "removeField", index: 0 }).selected).toBeNull();
  });

  it("duplicates below the original with a unique key and selects the copy", () => {
    const s = reduce(init(), { type: "duplicateField", index: 0 });
    expect(s.def.fields[1]).toEqual({ ...sampleForm().fields[0], key: "name_copy", label: "Full name (copy)" });
    expect(s.selected).toBe(1);
    expect(reduce(s, { type: "duplicateField", index: 0 }).def.fields[1].key).toBe("name_copy2");
  });

  it("moves fields and keeps the moved field selected", () => {
    let s = reduce(init(), { type: "select", index: 1 });
    s = reduce(s, { type: "moveField", index: 1, direction: 1 });
    expect(keys(s)).toEqual(["name", "role", "email", "portfolio"]);
    expect(s.selected).toBe(2);
  });

  it("moving past either end is a no-op", () => {
    const s0 = init();
    expect(reduce(s0, { type: "moveField", index: 0, direction: -1 })).toBe(s0);
    expect(reduce(s0, { type: "moveField", index: 3, direction: 1 })).toBe(s0);
  });
});

describe("replace and save", () => {
  it("replaces from YAML, normalising missing collections", () => {
    const s = reduce(init(), { type: "replace", def: { slug: "x", title: "X" } as never });
    expect(s.def).toEqual({ slug: "x", title: "X", settings: { public: false }, fields: [] });
    expect(s.dirty).toBe(true);
  });

  it("clears a selection that no longer exists after replace", () => {
    let s = reduce(init(), { type: "select", index: 3 });
    s = reduce(s, { type: "replace", def: { ...sampleForm(), fields: sampleForm().fields.slice(0, 2) } });
    expect(s.selected).toBeNull();
  });

  it("markSaved clears the dirty flag", () => {
    const s = reduce(reduce(init(), { type: "setMeta", patch: { title: "T" } }), { type: "markSaved" });
    expect(s.dirty).toBe(false);
  });
});

describe("helpers", () => {
  it("newFormDefinition is public, titled and empty", () => {
    expect(newFormDefinition()).toEqual({ slug: "", title: "Untitled form", settings: { public: true }, fields: [] });
  });

  it("newField picks the first free numbered key", () => {
    expect(newField("text", ["field1"]).key).toBe("field2");
  });

  it("normalizeForm drops non-object fields and repairs settings", () => {
    const def = normalizeForm({ slug: "a", title: "A", settings: "x", fields: [1, "x", { key: "a", type: "text", label: "A" }] });
    expect(def).toEqual({ slug: "a", title: "A", settings: { public: false }, fields: [{ key: "a", type: "text", label: "A" }] });
  });
});
