package jsbackend

const collectionRuntime = `
const __klang_char = (value) => Object.freeze({ __klang_char: String(value) });
const __klang_is_char = (value) => value !== null && typeof value === "object" && typeof value.__klang_char === "string";
const __klang_is_collection = (value) => value !== null && typeof value === "object" && (value.__klang_collection === "Map" || value.__klang_collection === "Table");
const __klang_collection_new = (kind, keyType = "T", valueType = "T") => ({ __klang_collection: kind, keyType, valueType, entries: new Map(), order: [], fallback: null });
const __klang_value_kind = (value) => {
    if (__klang_is_char(value)) return "Char";
    if (typeof value === "string") return "String";
    if (typeof value === "boolean") return "Bool";
    if (typeof value === "number") return Number.isInteger(value) ? "Int" : "Float";
    if (Array.isArray(value)) return "List";
    if (__klang_is_collection(value)) return value.__klang_collection;
    if (__klang_is_struct(value)) return value.__klang_struct;
    if (value === null) return "Null";
    return typeof value;
};
const __klang_type_matches = (value, type) => {
    if (!type || type === "T" || type === "Any") return true;
    if (type === "Char") return __klang_is_char(value);
    if (type === "String") return typeof value === "string";
    if (type === "Bool") return typeof value === "boolean";
    if (type === "Int") return typeof value === "number" && Number.isInteger(value);
    if (type === "UInt") return typeof value === "number" && Number.isInteger(value) && value >= 0;
    if (type === "Float") return typeof value === "number";
    if (type === "Table") return __klang_is_collection(value) && value.__klang_collection === "Table";
    if (type.startsWith("List[")) return Array.isArray(value);
    if (type.startsWith("Map[")) return __klang_is_collection(value) && value.__klang_collection === "Map";
    if (type.startsWith("Result[")) return __klang_is_result(value);
    return __klang_is_struct(value) && (value.__klang_struct === type || value.__klang_struct.startsWith(type + "["));
};
const __klang_key_info = (value, table) => {
    const kind = __klang_value_kind(value);
    if (!["String", "Int", "Float", "Bool", "Char"].includes(kind)) throw new TypeError(kind + " cannot be used as a " + (table ? "table" : "map") + " key");
    const raw = __klang_is_char(value) ? value.__klang_char : value;
    const repr = kind === "Bool" ? (raw ? "true" : "false") : String(raw);
    return { kind, repr, value: __klang_copy(value), token: table ? kind + ":" + repr : repr };
};
const __klang_collection_copy = (value) => {
    const copied = __klang_collection_new(value.__klang_collection, value.keyType, value.valueType);
    for (const token of value.order) {
        const entry = value.entries.get(token);
        copied.order.push(token);
        copied.entries.set(token, { kind: entry.kind, repr: entry.repr, key: __klang_copy(entry.key), value: __klang_copy(entry.value) });
    }
    copied.fallback = value.fallback ? __klang_collection_copy(value.fallback) : null;
    return copied;
};
const __klang_equal = (left, right) => {
    if (__klang_is_result(left) || __klang_is_result(right)) return __klang_is_result(left) && __klang_is_result(right) && left.ok === right.ok && __klang_equal(left.value, right.value);
    if (__klang_is_char(left) || __klang_is_char(right)) return __klang_is_char(left) && __klang_is_char(right) && left.__klang_char === right.__klang_char;
    if (Array.isArray(left) || Array.isArray(right)) return Array.isArray(left) && Array.isArray(right) && left.length === right.length && left.every((value, index) => __klang_equal(value, right[index]));
    if (__klang_is_collection(left) || __klang_is_collection(right)) {
        if (!__klang_is_collection(left) || !__klang_is_collection(right) || left.__klang_collection !== right.__klang_collection || left.entries.size !== right.entries.size) return false;
        for (const token of left.order) {
            const leftEntry = left.entries.get(token);
            const rightEntry = right.entries.get(token);
            if (!rightEntry || !__klang_equal(leftEntry.value, rightEntry.value)) return false;
        }
        return true;
    }
    if (__klang_is_struct(left) || __klang_is_struct(right)) {
        if (!__klang_is_struct(left) || !__klang_is_struct(right) || left.__klang_struct !== right.__klang_struct) return false;
        const leftFields = Object.keys(left).filter((field) => !field.startsWith("__"));
        const rightFields = Object.keys(right).filter((field) => !field.startsWith("__"));
        return leftFields.length === rightFields.length && leftFields.every((field) => Object.prototype.hasOwnProperty.call(right, field) && __klang_equal(left[field], right[field]));
    }
    return left === right;
};
const __klang_collection_put = (collection, key, value, operator = "=") => {
    const table = collection.__klang_collection === "Table";
    const info = __klang_key_info(key, table);
    if (!table && !__klang_type_matches(key, collection.keyType)) throw new TypeError("cannot use " + info.kind + " as map key type " + collection.keyType);
    if (!__klang_type_matches(value, collection.valueType)) throw new TypeError("cannot assign " + __klang_value_kind(value) + " to map value type " + collection.valueType);
    const existing = collection.entries.get(info.token);
    if (operator !== "=" && !existing) throw new RangeError("compound assignment requires existing " + (table ? "table" : "map") + " key " + JSON.stringify(info.repr));
    let next = __klang_copy(value);
    if (existing && operator === "+=") next = __klang_add(existing.value, next);
    else if (existing && operator === "-=") next = existing.value - next;
    else if (existing && operator === "*=") next = existing.value * next;
    else if (existing && operator === "/=") next = existing.value / next;
    else if (operator !== "=" && operator !== "+=" && operator !== "-=" && operator !== "*=" && operator !== "/=") throw new TypeError("unsupported assignment operator " + operator);
    if (!existing) collection.order.push(info.token);
    collection.entries.set(info.token, { kind: info.kind, repr: info.repr, key: info.value, value: __klang_copy(next) });
};
const __klang_collection_lookup = (collection, key) => {
    const table = collection.__klang_collection === "Table";
    const info = __klang_key_info(key, table);
    if (!table && !__klang_type_matches(key, collection.keyType)) throw new TypeError("cannot use " + info.kind + " as map key type " + collection.keyType);
    const own = collection.entries.get(info.token);
    if (own) return own;
    return collection.fallback ? __klang_collection_lookup(collection.fallback, key) : null;
};
const __klang_collection_get = (collection, key) => {
    const found = __klang_collection_lookup(collection, key);
    if (!found) {
        const info = __klang_key_info(key, collection.__klang_collection === "Table");
        throw new RangeError(collection.__klang_collection.toLowerCase() + " key " + JSON.stringify(info.repr) + " does not exist");
    }
    return __klang_copy(found.value);
};
const __klang_table_from_pairs = (pairs) => {
    const table = __klang_collection_new("Table");
    for (const pair of pairs) __klang_collection_put(table, pair[0], pair[1]);
    return table;
};
const __klang_as_map = (value, keyType, valueType) => {
    if (__klang_is_collection(value) && value.__klang_collection === "Map") {
        const copied = __klang_collection_copy(value);
        copied.keyType = keyType;
        copied.valueType = valueType;
        return copied;
    }
    if (!__klang_is_collection(value) || value.__klang_collection !== "Table") throw new TypeError("Map initialization expects a map literal");
    const result = __klang_collection_new("Map", keyType, valueType);
    for (const token of value.order) {
        const entry = value.entries.get(token);
        if (entry.kind !== "String") throw new TypeError(entry.kind + " table key cannot be used as a Map key");
        __klang_collection_put(result, entry.key, entry.value);
    }
    return result;
};
const __klang_table = (value) => {
    if (value === undefined) return __klang_collection_new("Table");
    if (!__klang_is_collection(value)) throw new TypeError("Table expects a map literal or Table value");
    if (value.__klang_collection === "Table") return __klang_collection_copy(value);
    const result = __klang_collection_new("Table");
    for (const token of value.order) {
        const entry = value.entries.get(token);
        __klang_collection_put(result, entry.key, entry.value);
    }
    return result;
};
const __klang_table_has = (table, key) => {
    if (!__klang_is_collection(table) || table.__klang_collection !== "Table") throw new TypeError("table_has expects a Table");
    return __klang_collection_lookup(table, key) !== null;
};
const __klang_table_delete = (table, key) => {
    if (!__klang_is_collection(table) || table.__klang_collection !== "Table") throw new TypeError("table_delete expects a Table");
    const result = __klang_collection_copy(table);
    const info = __klang_key_info(key, true);
    result.entries.delete(info.token);
    result.order = result.order.filter((token) => token !== info.token);
    return result;
};
const __klang_table_keys = (table) => table.order.map((token) => __klang_copy(table.entries.get(token).key));
const __klang_table_values = (table) => table.order.map((token) => __klang_copy(table.entries.get(token).value));
const __klang_table_entries = (table) => table.order.map((token) => {
    const entry = table.entries.get(token);
    return __klang_table_from_pairs([["key", entry.key], ["value", entry.value]]);
});
const __klang_table_sequence_count = (table) => {
    let count = 0;
    while (table.entries.has("Int:" + count)) count++;
    return count;
};
const __klang_table_set_fallback = (child, parent) => {
    if (!__klang_is_collection(child) || child.__klang_collection !== "Table" || !__klang_is_collection(parent) || parent.__klang_collection !== "Table") throw new TypeError("table_set_fallback expects two Tables");
    const result = __klang_collection_copy(child);
    result.fallback = __klang_collection_copy(parent);
    return result;
};
const __klang_collection_format = (value) => "{" + value.order.map((token) => { const entry = value.entries.get(token); return __klang_format(entry.key) + ": " + __klang_format(entry.value); }).join(", ") + "}";
const __klang_collection_json = (value) => {
    const entries = [];
    for (const token of value.order) {
        const entry = value.entries.get(token);
        if (entry.kind !== "String") throw new TypeError("JSON serialization requires String table keys");
        entries.push([entry.repr, __klang_to_json(entry.value)]);
    }
    entries.sort((left, right) => left[0].localeCompare(right[0]));
    return Object.fromEntries(entries);
};
`
