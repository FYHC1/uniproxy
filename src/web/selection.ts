// Change all filtered results, not merely the first rendered page; retain selections outside the scope.
export function selectScope(value:string[], scope:string[], checked:boolean):string[] {
  const ids = new Set(scope);
  return checked ? [...new Set([...value,...scope])] : value.filter(id => !ids.has(id));
}
